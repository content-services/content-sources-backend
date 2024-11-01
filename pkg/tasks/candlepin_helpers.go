package tasks

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	caliri "github.com/content-services/caliri/release/v4"
	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/candlepin_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/utils"
)

func GenContentDto(repo api.RepositoryResponse) caliri.ContentDTO {
	repoName := repo.Name
	id := candlepin_client.GetContentID(repo.UUID)
	repoType := candlepin_client.YumRepoType
	repoLabel := repo.Label
	repoVendor := getRepoVendor(repo)

	gpgKeyUrl := utils.Ptr("") // Default to "", meaning no gpg key url. For updating, nil means no update
	if repo.OrgID != config.RedHatOrg && repo.GpgKey != "" {
		gpgKeyUrl = models.CandlepinContentGpgKeyUrl(repo.OrgID, repo.UUID)
	}
	url := repo.URL
	if repo.Origin == config.OriginUpload {
		url = repo.LatestSnapshotURL
	}

	return caliri.ContentDTO{
		Id:         &id,
		Type:       &repoType,
		Label:      &repoLabel,
		Name:       &repoName,
		Vendor:     &repoVendor,
		GpgUrl:     gpgKeyUrl,
		ContentUrl: &url, // Set to upstream URL, but it is not used. Will use content overrides instead.
	}
}

// UnneededOverrides given a list of existing overrides, and expected overrides, return the existing overrides that are no longer needed
func UnneededOverrides(existingDtos []caliri.ContentOverrideDTO, expectedDTOs []caliri.ContentOverrideDTO) []caliri.ContentOverrideDTO {
	var toDelete []caliri.ContentOverrideDTO
	for i := 0; i < len(existingDtos); i++ {
		existing := existingDtos[i]
		found := false
		for j := 0; j < len(expectedDTOs); j++ {
			expectedDTO := expectedDTOs[j]
			if *existing.Name == *expectedDTO.Name && *existing.ContentLabel == *expectedDTO.ContentLabel {
				found = true
				break
			}
		}
		if !found {
			toDelete = append(toDelete, existing)
		}
	}
	return toDelete
}

// genOverrideDTOs uses the RepoConfigUUIDs to query the db and generate a mapping of content labels to distribution URLs
// for the snapshot within the template.  For all repos, we include an override for an 'empty' sslcacert, so it does not use the configured default
// on the client.  For custom repos, we override the base URL, due to the fact that we use different domains for RH and custom repos.
func GenOverrideDTO(ctx context.Context, daoReg *dao.DaoRegistry, orgId, domainName, contentPath string, template api.TemplateResponse) ([]caliri.ContentOverrideDTO, error) {
	mapping := []caliri.ContentOverrideDTO{}

	uuids := strings.Join(template.RepositoryUUIDS, ",")
	origins := strings.Join([]string{config.OriginExternal, config.OriginRedHat, config.OriginUpload}, ",")
	repos, _, err := daoReg.RepositoryConfig.List(ctx, orgId, api.PaginationData{Limit: -1}, api.FilterData{UUID: uuids, Origin: origins})
	if err != nil {
		return mapping, err
	}
	for _, repo := range repos.Data {
		repoOver, err := ContentOverridesForRepo(orgId, domainName, template.UUID, contentPath, repo)
		if err != nil {
			return mapping, err
		}
		mapping = append(mapping, repoOver...)
	}
	return mapping, nil
}

func RemoveUneededOverrides(ctx context.Context, cpClient candlepin_client.CandlepinClient, templateUUID string, expectedDTOs []caliri.ContentOverrideDTO) error {
	existingDtos, err := cpClient.FetchContentOverrides(ctx, templateUUID)
	if err != nil {
		return err
	}
	toDelete := UnneededOverrides(existingDtos, expectedDTOs)
	if len(toDelete) > 0 {
		err = cpClient.RemoveContentOverrides(ctx, templateUUID, toDelete)
		if err != nil {
			return err
		}
	}
	return nil
}

func ContentOverridesForRepo(orgId string, domainName string, templateUUID string, pulpContentPath string, repo api.RepositoryResponse) ([]caliri.ContentOverrideDTO, error) {
	mapping := []caliri.ContentOverrideDTO{}
	if repo.LastSnapshot == nil { // ignore repos without a snapshot
		return mapping, nil
	}

	mapping = append(mapping, caliri.ContentOverrideDTO{
		Name:         utils.Ptr(candlepin_client.OverrideNameCaCert),
		ContentLabel: &repo.Label,
		Value:        utils.Ptr(" "), // use a single space because candlepin doesn't allow "" or null
	})
	// Disable OCSP checking, as aws doesn't support it?
	mapping = append(mapping, caliri.ContentOverrideDTO{
		Name:         utils.Ptr(candlepin_client.OverrideSSLVerifyStatus),
		ContentLabel: &repo.Label,
		Value:        utils.Ptr("0"),
	})

	if repo.OrgID == orgId { // Don't override RH repo baseurls
		distPath := customTemplateSnapshotPath(templateUUID, repo.UUID)
		path, err := url.JoinPath(pulpContentPath, domainName, distPath)
		if err != nil {
			return mapping, err
		}
		mapping = append(mapping, caliri.ContentOverrideDTO{
			Name:         utils.Ptr(candlepin_client.OverrideNameBaseUrl),
			ContentLabel: &repo.Label,
			Value:        &path,
		})
		if repo.ModuleHotfixes {
			mapping = append(mapping, caliri.ContentOverrideDTO{
				Name:         utils.Ptr(candlepin_client.OverrideModuleHotfixes),
				ContentLabel: &repo.Label,
				Value:        utils.Ptr("1"),
			})
		}
	}
	return mapping, nil
}

func customTemplateSnapshotPath(templateUUID string, repoUUID string) string {
	return fmt.Sprintf("templates/%v/%v", templateUUID, repoUUID)
}
