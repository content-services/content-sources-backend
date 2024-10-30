package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	caliri "github.com/content-services/caliri/release/v4"
	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/candlepin_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/tasks/helpers"
	"github.com/content-services/content-sources-backend/pkg/tasks/payloads"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"golang.org/x/exp/slices"
)

func lookupTemplate(ctx context.Context, daoReg *dao.DaoRegistry, orgId string, templateUUID string) (*api.TemplateResponse, error) {
	template, err := daoReg.Template.Fetch(ctx, orgId, templateUUID, true)
	if err != nil {
		return nil, err
	}
	if template.DeletedAt.Valid {
		return nil, nil
	}
	return &template, nil
}

func UpdateTemplateContentHandler(ctx context.Context, task *models.TaskInfo, queue *queue.Queue) error {
	opts := payloads.UpdateTemplateContentPayload{}
	if err := json.Unmarshal(task.Payload, &opts); err != nil {
		return fmt.Errorf("payload incorrect type for UpdateTemplateDistributions")
	}

	logger := LogForTask(task.Id.String(), task.Typename, task.RequestID)
	ctxWithLogger := logger.WithContext(ctx)

	daoReg := dao.GetDaoRegistry(db.DB)

	template, err := lookupTemplate(ctxWithLogger, daoReg, task.OrgId, opts.TemplateUUID)
	if template == nil || err != nil {
		return err
	}

	domainName, err := daoReg.Domain.Fetch(ctxWithLogger, task.OrgId)
	if err != nil {
		return err
	}

	rhDomainName, err := daoReg.Domain.Fetch(ctxWithLogger, config.RedHatOrg)
	if err != nil {
		return err
	}

	pulpClient := pulp_client.GetPulpClientWithDomain(domainName)
	cpClient := candlepin_client.NewCandlepinClient()

	t := UpdateTemplateContent{
		orgId:          task.OrgId,
		template:       *template,
		domainName:     domainName,
		rhDomainName:   rhDomainName,
		repositoryUUID: task.ObjectUUID,
		daoReg:         daoReg,
		pulpClient:     pulpClient,
		cpClient:       cpClient,
		task:           task,
		payload:        &opts,
		queue:          queue,
		ctx:            ctxWithLogger,
		logger:         logger,
	}

	err = t.daoReg.Template.UpdateLastError(t.ctx, template.OrgID, template.UUID, "")
	if err != nil {
		return err
	}

	// By creating the environment first, we don't block on pulp
	env, err := t.RunEnvironmentCreate()
	if err != nil {
		return err
	}

	err = t.RunPulp()
	if err != nil {
		return err
	}
	return t.RunCandlepin(env)
}

type UpdateTemplateContent struct {
	orgId          string
	domainName     string
	rhDomainName   string
	template       api.TemplateResponse
	repositoryUUID uuid.UUID
	daoReg         *dao.DaoRegistry
	pulpClient     pulp_client.PulpClient
	cpClient       candlepin_client.CandlepinClient
	payload        *payloads.UpdateTemplateContentPayload
	task           *models.TaskInfo
	queue          *queue.Queue
	ctx            context.Context
	logger         *zerolog.Logger
}

// RunPulp creates (when a repository is added), updates (if the template date has changed), or removes (when a repository is removed) pulp distributions
// when a template is created or updated. Each distribution is under a path that is based on the template uuid. It serves the latest snapshot content up to the
// date set on the template.
func (t *UpdateTemplateContent) RunPulp() error {
	if t.payload.RepoConfigUUIDs == nil {
		return nil
	}

	repoConfigDistributionHref := map[string]string{} // mapping to associate each repo config to a distribution href

	reposAdded, reposRemoved, reposUnchanged, allRepos, err := t.daoReg.Template.GetRepoChanges(t.ctx, t.payload.TemplateUUID, t.payload.RepoConfigUUIDs)
	if err != nil {
		return fmt.Errorf("error getting repo changes: %w", err)
	}

	var templateDate time.Time
	if t.template.UseLatest {
		templateDate = time.Now()
	} else {
		templateDate = t.template.Date
	}

	l := api.ListSnapshotByDateRequest{Date: templateDate, RepositoryUUIDS: allRepos}
	snapshots, err := t.daoReg.Snapshot.FetchSnapshotsModelByDateAndRepository(t.ctx, t.orgId, l)
	if err != nil {
		return err
	}

	if reposAdded != nil {
		err := t.handleReposAdded(reposAdded, snapshots, repoConfigDistributionHref)
		if err != nil {
			return err
		}
	}

	if reposRemoved != nil {
		err := t.handleReposRemoved(reposRemoved)
		if err != nil {
			return err
		}
		keepRepoConfigUUIDs := append(reposUnchanged, reposAdded...)
		err = t.daoReg.Template.DeleteTemplateRepoConfigs(t.ctx, t.payload.TemplateUUID, keepRepoConfigUUIDs)
		if err != nil {
			return fmt.Errorf("error in DeleteTemplateRepoConfigs: %w", err)
		}
	}

	if reposUnchanged != nil {
		err := t.handleReposUnchanged(reposUnchanged, snapshots, repoConfigDistributionHref)
		if err != nil {
			return err
		}
	}

	err = t.daoReg.Template.UpdateDistributionHrefs(t.ctx, t.payload.TemplateUUID, t.payload.RepoConfigUUIDs, snapshots, repoConfigDistributionHref)
	if err != nil {
		return fmt.Errorf("error updating distribution hrefs: %w", err)
	}

	return nil
}

func (t *UpdateTemplateContent) handleReposAdded(reposAdded []string, snapshots []models.Snapshot, repoConfigDistributionHref map[string]string) error {
	for _, repoConfigUUID := range reposAdded {
		repo, err := t.daoReg.RepositoryConfig.Fetch(t.ctx, t.orgId, repoConfigUUID)
		if err != nil {
			return err
		}
		if repo.LastSnapshot == nil {
			continue
		}

		// Configure client for org
		if repo.OrgID == config.RedHatOrg {
			t.pulpClient = t.pulpClient.WithDomain(t.rhDomainName)
		} else {
			t.pulpClient = t.pulpClient.WithDomain(t.domainName)
		}

		snapIndex := slices.IndexFunc(snapshots, func(s models.Snapshot) bool {
			return s.RepositoryConfigurationUUID == repoConfigUUID
		})

		distPath, distName, err := getDistPathAndName(repo, t.payload.TemplateUUID)
		if err != nil {
			return err
		}

		distResp, err := helpers.NewPulpDistributionHelper(t.ctx, t.pulpClient).CreateDistribution(repo.OrgID, snapshots[snapIndex].PublicationHref, distName, distPath)
		if err != nil {
			return err
		}

		distHrefPtr := pulp_client.SelectRpmDistributionHref(distResp)
		if distHrefPtr == nil {
			return fmt.Errorf("could not find a distribution href in task: %v", *distResp.PulpHref)
		}

		repoConfigDistributionHref[repoConfigUUID] = *distHrefPtr
	}
	return nil
}

func (t *UpdateTemplateContent) handleReposUnchanged(reposUnchanged []string, snapshots []models.Snapshot, repoConfigDistributionHref map[string]string) error {
	for _, repoConfigUUID := range reposUnchanged {
		repo, err := t.daoReg.RepositoryConfig.Fetch(t.ctx, t.orgId, repoConfigUUID)
		if err != nil {
			return err
		}
		if repo.LastSnapshot == nil {
			continue
		}

		// Configure client for org
		if repo.OrgID == config.RedHatOrg {
			t.pulpClient = t.pulpClient.WithDomain(t.rhDomainName)
		} else {
			t.pulpClient = t.pulpClient.WithDomain(t.domainName)
		}

		snapIndex := slices.IndexFunc(snapshots, func(s models.Snapshot) bool {
			return s.RepositoryConfigurationUUID == repoConfigUUID
		})
		if snapIndex == -1 {
			continue
		}

		distPath, distName, err := getDistPathAndName(repo, t.payload.TemplateUUID)
		if err != nil {
			return err
		}

		_, _, err = helpers.NewPulpDistributionHelper(t.ctx, t.pulpClient).CreateOrUpdateDistribution(repo.OrgID, distName, distPath, snapshots[snapIndex].PublicationHref)
		if err != nil {
			return err
		}

		distResp, err := t.pulpClient.FindDistributionByPath(t.ctx, distPath)
		if err != nil {
			return err
		}
		repoConfigDistributionHref[repoConfigUUID] = *distResp.PulpHref
	}
	return nil
}

func (t *UpdateTemplateContent) handleReposRemoved(reposRemoved []string) error {
	for _, repoConfigUUID := range reposRemoved {
		repo, err := t.daoReg.RepositoryConfig.Fetch(t.ctx, t.orgId, repoConfigUUID)
		if err != nil {
			return err
		}
		if repo.LastSnapshot == nil {
			continue
		}

		// Configure client for org
		if repo.OrgID == config.RedHatOrg {
			t.pulpClient = t.pulpClient.WithDomain(t.rhDomainName)
		} else {
			t.pulpClient = t.pulpClient.WithDomain(t.domainName)
		}

		distHref, err := t.daoReg.Template.GetDistributionHref(t.ctx, t.payload.TemplateUUID, repoConfigUUID)
		if err != nil {
			return err
		}
		taskHref, err := t.pulpClient.DeleteRpmDistribution(t.ctx, distHref)
		if err != nil {
			return err
		}

		if taskHref != "" {
			_, err = t.pulpClient.PollTask(t.ctx, taskHref)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func getDistPathAndName(repo api.RepositoryResponse, templateUUID string) (distPath string, distName string, err error) {
	if repo.OrgID == config.RedHatOrg {
		path, err := getRHRepoContentPath(repo.URL)
		if err != nil {
			return "", "", err
		}
		distPath = fmt.Sprintf("templates/%v/%v", templateUUID, path)
	} else {
		distPath = customTemplateSnapshotPath(templateUUID, repo.UUID)
	}

	distName = templateUUID + "/" + repo.UUID
	return distPath, distName, nil
}

func getRHRepoContentPath(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	return u.Path[1 : len(u.Path)-1], nil
}

func (t *UpdateTemplateContent) RunEnvironmentCreate() (*caliri.EnvironmentDTO, error) {
	rhContentPath, err := t.pulpClient.GetContentPath(t.ctx)
	if err != nil {
		return nil, err
	}
	prefix, err := url.JoinPath(rhContentPath, t.rhDomainName, "templates", t.payload.TemplateUUID)
	if err != nil {
		return nil, err
	}
	env, err := t.fetchOrCreateEnvironment(prefix)
	if err != nil {
		return nil, err
	}
	if !t.template.RHSMEnvironmentCreated {
		err := t.daoReg.Template.SetEnvironmentCreated(t.ctx, t.template.UUID)
		if err != nil {
			return nil, err
		}
	}

	return env, nil
}

// RunCandlepin creates an environment for the template and content sets for each repository.
// Each content set's URL is the distribution URL created during RunPulp().
// May promote or demote content from the environment, depending on if the repository is being added or removed from template.
// If not created for the given org previously, this will also create a product and pool.
func (t *UpdateTemplateContent) RunCandlepin(env *caliri.EnvironmentDTO) error {
	var err error

	err = t.cpClient.CreateProduct(t.ctx, t.orgId)
	if err != nil {
		return err
	}

	poolID, err := t.cpClient.CreatePool(t.ctx, t.orgId)
	if err != nil {
		return err
	}
	t.payload.PoolID = &poolID
	err = t.updatePayload()
	if err != nil {
		return err
	}

	customContent, customContentIDs, rhContentIDs, err := t.getContentList()
	if err != nil {
		return err
	}
	// Instead of figuring out which custom contents already exist, we just call create for all
	for _, item := range customContent {
		err = t.cpClient.CreateContent(t.ctx, t.orgId, item)
		if err != nil {
			return err
		}
	}

	// TODO we can use create content batch when the api spec is fixed
	// err = c.client.CreateContentBatch(candlepin_client.DevelOrgKey, content)
	// if err != nil {
	//	return err
	// }

	err = t.cpClient.AddContentBatchToProduct(t.ctx, t.orgId, customContentIDs)
	if err != nil {
		return err
	}

	env, err = t.renameEnvironmentIfNeeded(env)
	if err != nil {
		return err
	}

	envContent := env.GetEnvironmentContent()
	var contentInEnv []string
	contentIDs := append(customContentIDs, rhContentIDs...)
	for _, content := range envContent {
		contentInEnv = append(contentInEnv, content.GetContentId())
	}
	contentToPromote := difference(contentIDs, contentInEnv)
	contentToDemote := difference(contentInEnv, contentIDs)

	if len(contentToPromote) != 0 {
		err = t.promoteContent(contentToPromote)
		if err != nil {
			return err
		}
	}

	if len(contentToDemote) != 0 {
		err = t.demoteContent(contentToDemote)
		if err != nil {
			return err
		}
	}

	rhContentPath, err := t.pulpClient.GetContentPath(t.ctx)
	if err != nil {
		return err
	}
	overrideDtos, err := t.genOverrideDTOs(rhContentPath)
	if err != nil {
		return err
	}

	err = t.removeUnneededOverrides(overrideDtos)
	if err != nil {
		return err
	}

	err = t.cpClient.UpdateContentOverrides(t.ctx, t.payload.TemplateUUID, overrideDtos)
	if err != nil {
		return err
	}

	return nil
}

func (t *UpdateTemplateContent) fetchOrCreateEnvironment(prefix string) (*caliri.EnvironmentDTO, error) {
	env, err := t.cpClient.FetchEnvironment(t.ctx, t.payload.TemplateUUID)
	if err != nil {
		return nil, err
	}
	if env != nil {
		return env, nil
	}

	env, err = t.cpClient.CreateEnvironment(t.ctx, t.orgId, t.template.Name, t.template.UUID, prefix)
	if err != nil {
		return nil, err
	}
	return env, nil
}

func (t *UpdateTemplateContent) renameEnvironmentIfNeeded(env *caliri.EnvironmentDTO) (*caliri.EnvironmentDTO, error) {
	template := t.template
	if template.Name == *env.Name {
		return env, nil
	}

	renamed, err := t.cpClient.RenameEnvironment(t.ctx, template.UUID, template.Name)
	if err != nil {
		return nil, err
	}

	return renamed, nil
}

func (t *UpdateTemplateContent) promoteContent(repoConfigUUIDs []string) error {
	err := t.cpClient.PromoteContentToEnvironment(t.ctx, t.payload.TemplateUUID, repoConfigUUIDs)
	if err != nil {
		return err
	}
	return nil
}

func (t *UpdateTemplateContent) demoteContent(repoConfigUUIDs []string) error {
	err := t.cpClient.DemoteContentFromEnvironment(t.ctx, t.payload.TemplateUUID, repoConfigUUIDs)
	if err != nil {
		return err
	}
	return nil
}

// getContentList return the list of ContentDTO that will be created in Candlepin.
// Returns list of custom content to be created, a list of custom content IDs, a list of red hat content IDs, and an error.
func (t *UpdateTemplateContent) getContentList() ([]caliri.ContentDTO, []string, []string, error) {
	uuids := strings.Join(t.payload.RepoConfigUUIDs, ",")
	repoConfigs, _, err := t.daoReg.RepositoryConfig.List(t.ctx, t.orgId, api.PaginationData{Limit: -1}, api.FilterData{UUID: uuids, Origin: config.OriginExternal})
	if err != nil {
		return nil, nil, nil, err
	}

	rhRepos, _, err := t.daoReg.RepositoryConfig.List(t.ctx, t.orgId, api.PaginationData{Limit: -1}, api.FilterData{UUID: uuids, Origin: config.OriginRedHat})
	if err != nil {
		return nil, nil, nil, err
	}

	rhContentIDs, err := t.getRedHatContentIDs(rhRepos.Data)
	if err != nil {
		return nil, nil, nil, err
	}
	contentToCreate, customContentIDs := createContentItems(repoConfigs.Data)

	return contentToCreate, customContentIDs, rhContentIDs, nil
}

// genOverrideDTOs uses the RepoConfigUUIDs to query the db and generate a mapping of content labels to distribution URLs
// for the snapshot within the template.  For all repos, we include an override for an 'empty' sslcacert, so it does not use the configured default
// on the client.  For custom repos, we override the base URL, due to the fact that we use different domains for RH and custom repos.
func (t *UpdateTemplateContent) genOverrideDTOs(contentPath string) ([]caliri.ContentOverrideDTO, error) {
	mapping := []caliri.ContentOverrideDTO{}
	uuids := strings.Join(t.payload.RepoConfigUUIDs, ",")
	origins := strings.Join([]string{config.OriginExternal, config.OriginRedHat}, ",")
	customRepos, _, err := t.daoReg.RepositoryConfig.List(t.ctx, t.orgId, api.PaginationData{Limit: -1}, api.FilterData{UUID: uuids, Origin: origins})
	if err != nil {
		return mapping, err
	}
	for i := 0; i < len(customRepos.Data); i++ {
		repoOver, err := ContentOverridesForRepo(t.orgId, t.domainName, t.payload.TemplateUUID, contentPath, customRepos.Data[i])
		if err != nil {
			return mapping, err
		}
		mapping = append(mapping, repoOver...)
	}
	return mapping, nil
}

func (t *UpdateTemplateContent) removeUnneededOverrides(expectedDTOs []caliri.ContentOverrideDTO) error {
	existingDtos, err := t.cpClient.FetchContentOverrides(t.ctx, t.payload.TemplateUUID)
	if err != nil {
		return err
	}
	toDelete := UnneededOverrides(existingDtos, expectedDTOs)
	if len(toDelete) > 0 {
		err = t.cpClient.RemoveContentOverrides(t.ctx, t.payload.TemplateUUID, toDelete)
		if err != nil {
			return err
		}
	}
	return nil
}

// getRedHatContentIDs returns a list of red hat repo candlepin content IDs for each red hat repo in rhRepos, matched by label
func (t *UpdateTemplateContent) getRedHatContentIDs(rhRepos []api.RepositoryResponse) ([]string, error) {
	labels := []string{}
	for _, rhRepo := range rhRepos {
		labels = append(labels, rhRepo.Label)
	}
	contents, err := t.cpClient.FetchContentsByLabel(t.ctx, t.orgId, labels)
	if err != nil {
		return []string{}, err
	}

	ids := []string{}
	for _, content := range contents {
		ids = append(ids, *content.Id)
	}
	return ids, nil
}

func (t *UpdateTemplateContent) updatePayload() error {
	var err error
	a := *t.payload
	t.task, err = (*t.queue).UpdatePayload(t.task, a)
	if err != nil {
		return err
	}
	return nil
}

func createContentItems(repos []api.RepositoryResponse) ([]caliri.ContentDTO, []string) {
	var content []caliri.ContentDTO
	var contentIDs []string

	for _, repo := range repos {
		if repo.OrgID == config.RedHatOrg {
			continue
		}
		if repo.LastSnapshot == nil {
			continue
		}
		repoContent := GenContentDto(repo)
		content = append(content, repoContent)
		contentIDs = append(contentIDs, *repoContent.Id)
	}
	return content, contentIDs
}

func getRepoVendor(repo api.RepositoryResponse) string {
	if repo.OrgID == config.RedHatOrg {
		return "Red Hat"
	} else {
		return "Custom"
	}
}

// difference returns slice that contains elements of a that are not in b
func difference(a, b []string) []string {
	mb := make(map[string]struct{}, len(b))
	for _, x := range b {
		mb[x] = struct{}{}
	}
	var diff []string
	for _, x := range a {
		if _, found := mb[x]; !found {
			diff = append(diff, x)
		}
	}
	return diff
}
