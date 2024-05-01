package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"

	caliri "github.com/content-services/caliri/release/v4"
	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/candlepin_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/tasks/payloads"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	zest "github.com/content-services/zest/release/v2024"
	"github.com/google/uuid"
	"github.com/labstack/gommon/random"
	"github.com/rs/zerolog"
	"golang.org/x/exp/slices"
	"regexp"
	"strings"
)

func UpdateTemplateContentHandler(ctx context.Context, task *models.TaskInfo, queue *queue.Queue) error {
	opts := payloads.UpdateTemplateContentPayload{}
	if err := json.Unmarshal(task.Payload, &opts); err != nil {
		return fmt.Errorf("payload incorrect type for UpdateTemplateDistributions")
	}

	logger := LogForTask(task.Id.String(), task.Typename, task.RequestID)
	ctxWithLogger := logger.WithContext(ctx)

	daoReg := dao.GetDaoRegistry(db.DB)
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
		ownerKey:       candlepin_client.DevelOrgKey,
		domainName:     domainName,
		rhDomainName:   rhDomainName,
		repositoryUUID: task.RepositoryUUID,
		daoReg:         daoReg,
		pulpClient:     pulpClient,
		cpClient:       cpClient,
		task:           task,
		payload:        &opts,
		queue:          queue,
		ctx:            ctx,
		logger:         logger,
	}

	err = t.RunPulp()
	if err != nil {
		return err
	}
	return t.RunCandlepin()
}

type UpdateTemplateContent struct {
	orgId          string
	ownerKey       string
	domainName     string
	rhDomainName   string
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
		return err
	}

	template, err := t.daoReg.Template.Fetch(t.ctx, t.orgId, t.payload.TemplateUUID)
	if err != nil {
		return err
	}

	l := api.ListSnapshotByDateRequest{Date: api.Date(template.Date), RepositoryUUIDS: allRepos}
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
			return err
		}
	}

	if reposUnchanged != nil {
		err := t.handleReposUnchanged(reposUnchanged, snapshots, repoConfigDistributionHref)
		if err != nil {
			return err
		}
	}

	err = t.daoReg.Template.UpdateDistributionHrefs(t.ctx, t.payload.TemplateUUID, t.payload.RepoConfigUUIDs, repoConfigDistributionHref)
	if err != nil {
		return err
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

		distPath, distName, err := getDistPathAndName(repo, t.payload.TemplateUUID, snapshots[snapIndex].UUID)
		if err != nil {
			return err
		}

		distResp, err := t.createDistributionWithContentGuard(snapshots[snapIndex].PublicationHref, distName, distPath)
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

		distPath, distName, err := getDistPathAndName(repo, t.payload.TemplateUUID, snapshots[snapIndex].UUID)
		if err != nil {
			return err
		}

		distHref, err := t.daoReg.Template.GetDistributionHref(t.ctx, t.payload.TemplateUUID, repoConfigUUID)
		if err != nil {
			return err
		}

		err = t.createOrUpdateDistribution(distHref, distName, distPath, snapshots[snapIndex].PublicationHref)
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

func (t *UpdateTemplateContent) createDistributionWithContentGuard(publicationHref, distName, distPath string) (*zest.TaskResponse, error) {
	// Create content guard
	var contentGuardHref *string
	if t.orgId != config.RedHatOrg && config.Get().Clients.Pulp.CustomRepoContentGuards {
		href, err := t.pulpClient.CreateOrUpdateGuardsForOrg(t.ctx, t.orgId)
		if err != nil {
			return nil, fmt.Errorf("could not fetch/create/update content guard: %w", err)
		}
		contentGuardHref = &href
	}

	// Create distribution
	distTask, err := t.pulpClient.CreateRpmDistribution(t.ctx, publicationHref, distName, distPath, contentGuardHref)
	if err != nil {
		return nil, err
	}

	distResp, err := t.pulpClient.PollTask(t.ctx, *distTask)
	if err != nil {
		return nil, err
	}

	return distResp, nil
}

func (t *UpdateTemplateContent) createOrUpdateDistribution(distHref, distName, distPath, publicationHref string) error {
	resp, err := t.pulpClient.FindDistributionByPath(t.ctx, distPath)
	if err != nil {
		return err
	}

	if resp == nil {
		_, err := t.createDistributionWithContentGuard(publicationHref, distName, distPath)
		if err != nil {
			return err
		}
	} else {
		taskHref, err := t.pulpClient.UpdateRpmDistribution(t.ctx, distHref, publicationHref, distName, distPath)
		if err != nil {
			return err
		}

		_, err = t.pulpClient.PollTask(t.ctx, taskHref)
		if err != nil {
			return err
		}
	}
	return nil
}

func getDistPathAndName(repo api.RepositoryResponse, templateUUID string, snapshotUUID string) (distPath string, distName string, err error) {
	if repo.OrgID == config.RedHatOrg {
		path, err := getRHRepoContentPath(repo.URL)
		if err != nil {
			return "", "", err
		}
		distPath = fmt.Sprintf("templates/%v/%v", templateUUID, path)
	} else {
		distPath = fmt.Sprintf("templates/%v/%v", templateUUID, repo.UUID)
	}

	distName = templateUUID + "/" + snapshotUUID
	return distPath, distName, nil
}

func getRHRepoContentPath(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	return u.Path[1 : len(u.Path)-1], nil
}

// RunCandlepin creates an environment for the template and content sets for each repository.
// Each content set's URL is the distribution URL created during RunPulp().
// May promote or demote content from the environment, depending on if the repository is being added or removed from template.
// If not created for the given org previously, this will also create a product and pool.
func (t *UpdateTemplateContent) RunCandlepin() error {
	var err error

	err = t.cpClient.CreateProduct(t.ctx, t.ownerKey)
	if err != nil {
		return err
	}

	poolID, err := t.cpClient.CreatePool(t.ctx, t.ownerKey)
	if err != nil {
		return err
	}
	t.payload.PoolID = &poolID
	err = t.updatePayload()
	if err != nil {
		return err
	}

	content, contentIDs, err := t.getContentList()
	if err != nil {
		return err
	}

	for _, item := range content {
		err = t.cpClient.CreateContent(t.ctx, t.ownerKey, item)
		if err != nil {
			return err
		}
	}

	// TODO we can use create content batch when the api spec is fixed
	//err = c.client.CreateContentBatch(candlepin_client.DevelOrgKey, content)
	//if err != nil {
	//	return err
	//}

	err = t.cpClient.AddContentBatchToProduct(t.ctx, t.ownerKey, contentIDs)
	if err != nil {
		return err
	}

	contentPath, err := t.pulpClient.GetContentPath(t.ctx)
	if err != nil {
		return err
	}
	prefix := contentPath + t.rhDomainName + "/templates/" + t.payload.TemplateUUID

	envID := candlepin_client.GetEnvironmentID(t.payload.TemplateUUID)
	env, err := t.fetchOrCreateEnvironment(envID, prefix)
	if err != nil {
		return err
	}

	envContent := env.GetEnvironmentContent()
	var contentInEnv []string
	for _, content := range envContent {
		contentInEnv = append(contentInEnv, content.GetContentId())
	}
	contentToPromote := difference(contentIDs, contentInEnv)
	contentToDemote := difference(contentInEnv, contentIDs)

	if len(contentToPromote) != 0 {
		err = t.promoteContent(contentToPromote, envID)
		if err != nil {
			return err
		}
	}

	if len(contentToDemote) != 0 {
		err = t.demoteContent(contentToDemote, envID)
		if err != nil {
			return err
		}
	}

	return nil
}

func (t *UpdateTemplateContent) fetchOrCreateEnvironment(envID string, prefix string) (*caliri.EnvironmentDTO, error) {
	env, err := t.cpClient.FetchEnvironment(t.ctx, envID)
	if err != nil && !strings.Contains(err.Error(), "couldn't fetch environment: 404:") {
		return nil, err
	}
	if env != nil {
		return env, nil
	}
	env, err = t.cpClient.CreateEnvironment(t.ctx, t.ownerKey, envID, envID, prefix)
	if err != nil {
		return nil, err
	}
	return env, nil
}

func (t *UpdateTemplateContent) promoteContent(reposAdded []string, envID string) error {
	var addedIDs []string
	for _, repoUUID := range reposAdded {
		addedIDs = append(addedIDs, candlepin_client.GetContentID(repoUUID))
	}
	err := t.cpClient.PromoteContentToEnvironment(t.ctx, envID, addedIDs)
	if err != nil {
		return err
	}
	return nil
}

func (t *UpdateTemplateContent) demoteContent(reposRemoved []string, envID string) error {
	var removedIDs []string
	for _, repoUUID := range reposRemoved {
		removedIDs = append(removedIDs, candlepin_client.GetContentID(repoUUID))
	}
	err := t.cpClient.DemoteContentFromEnvironment(t.ctx, envID, removedIDs)
	if err != nil {
		return err
	}
	return nil
}

// getContentList return the list of ContentDTO that will be created in Candlepin
func (t *UpdateTemplateContent) getContentList() ([]caliri.ContentDTO, []string, error) {
	uuids := strings.Join(t.payload.RepoConfigUUIDs, ",")
	repos, _, err := t.daoReg.RepositoryConfig.List(t.ctx, t.orgId, api.PaginationData{Limit: -1}, api.FilterData{UUID: uuids, Origin: config.OriginRedHat + "," + config.OriginExternal})
	if err != nil {
		return nil, nil, err
	}

	repoLabels, err := t.getRepoLabels(repos.Data)
	if err != nil {
		return nil, nil, err
	}
	return createContentItems(repos.Data, repoLabels)
}

func (t *UpdateTemplateContent) getRepoLabels(requestedContent []api.RepositoryResponse) ([]string, error) {
	contentLabels, contentIDs, err := t.cpClient.ListContents(t.ctx, t.ownerKey)
	if err != nil {
		return nil, err
	}

	var labels []string
	for _, reqRepo := range requestedContent {
		reqLabel, err := getRepoLabel(reqRepo, false)
		reqID := candlepin_client.GetContentID(reqRepo.UUID)
		if err != nil {
			return nil, err
		}
		// If the label exists, but the content is different, we must add randomization to the label
		if (slices.Contains(contentLabels, reqLabel) && !slices.Contains(contentIDs, reqID)) || slices.Contains(labels, reqLabel) {
			reqLabel, err = getRepoLabel(reqRepo, true)
			if err != nil {
				return nil, err
			}
			labels = append(labels, reqLabel)
		} else {
			labels = append(labels, reqLabel)
		}
	}
	return labels, nil
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

func createContentItems(repos []api.RepositoryResponse, repoLabels []string) ([]caliri.ContentDTO, []string, error) {
	var content []caliri.ContentDTO
	var contentIDs []string
	var err error

	for i, repo := range repos {
		if repo.LastSnapshot == nil {
			continue
		}
		repoName := repo.Name
		id := candlepin_client.GetContentID(repo.UUID)
		repoType := candlepin_client.YumRepoType
		repoLabel := repoLabels[i]
		repoVendor := getRepoVendor(repo)

		var contentURL string // TODO nothing is set for custom repos. must use content overrides to set baseurl instead.
		if repo.OrgID == config.RedHatOrg {
			contentURL, err = getRHRepoContentPath(repo.URL)
			if err != nil {
				return nil, nil, err
			}
		}

		content = append(content, caliri.ContentDTO{
			Id:         &id,
			Type:       &repoType,
			Label:      &repoLabel,
			Name:       &repoName,
			Vendor:     &repoVendor,
			ContentUrl: &contentURL,
		})
		contentIDs = append(contentIDs, id)
	}
	return content, contentIDs, nil
}

func getRepoLabel(repo api.RepositoryResponse, randomize bool) (string, error) {
	// Replace any nonalphanumeric characters with an underscore
	// e.g: "!!my repo?test15()" => "__my_repo_test15__"
	re, err := regexp.Compile(`[^a-zA-Z0-9:space]`)
	if err != nil {
		return "", err
	}

	var repoLabel string
	if repo.OrgID == config.RedHatOrg {
		repoLabel = repo.Label
	} else {
		repoLabel = re.ReplaceAllString(repo.Name, "_")
	}

	if randomize {
		repoLabel = repoLabel + "_" + random.String(10, random.Alphabetic)
	}

	return repoLabel, nil
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
