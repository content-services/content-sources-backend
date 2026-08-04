package tasks

import (
	"context"
	"fmt"
	"net/url"

	caliri "github.com/content-services/caliri/release/v4"
	"github.com/content-services/content-sources-backend/pkg/api"
	candlepin_client "github.com/content-services/content-sources-backend/pkg/clients/candlepin_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/utils"
)

func GenContentDto(repo api.RepositoryResponse) caliri.ContentDTO {
	contentURL := repo.URL
	if repo.Origin == config.OriginUpload {
		contentURL = repo.LatestSnapshotURL
	}
	return genContentDto(repo, contentURL)
}

func genContentDto(repo api.RepositoryResponse, contentURL string) caliri.ContentDTO {
	repoName := repo.Name
	id := candlepin_client.GetContentID(repo.UUID)
	repoType := candlepin_client.YumRepoType
	repoLabel := repo.Label
	repoVendor := getRepoVendor(repo)

	gpgKeyUrl := utils.Ptr("") // Default to "", meaning no gpg key url. For updating, nil means no update
	if repo.OrgID != config.RedHatOrg && repo.GpgKey != "" {
		gpgKeyUrl = models.CandlepinContentGpgKeyUrl(repo.OrgID, repo.UUID)
	}
	return caliri.ContentDTO{
		Id:         &id,
		Type:       &repoType,
		Label:      &repoLabel,
		Name:       &repoName,
		Vendor:     &repoVendor,
		GpgUrl:     gpgKeyUrl,
		ContentUrl: &contentURL, // Set to upstream URL, but it is not used. Will use content overrides instead.
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

// GenOverrideDTO loads repository configs by UUID and returns Candlepin content override DTOs for the template snapshot (e.g. sslcacert, base URL).
//
// repoConfigUUIDs must be non-empty (UpdateTemplateContentHandler enforces this).
func GenOverrideDTO(ctx context.Context, daoReg *dao.DaoRegistry, orgId, domainName, rhDomainName, communityDomainName, contentPath string, template api.TemplateResponse, repoConfigUUIDs []string) ([]caliri.ContentOverrideDTO, error) {
	mapping := []caliri.ContentOverrideDTO{}

	if len(repoConfigUUIDs) == 0 {
		return nil, fmt.Errorf("refusing to list repository configs for template %s: no repository config UUIDs (unfiltered list would include unrelated orgs)", template.UUID)
	}
	for _, repoConfigUUID := range repoConfigUUIDs {
		repo, err := daoReg.RepositoryConfig.FetchWithoutOrgID(ctx, repoConfigUUID, false)
		if err != nil {
			return mapping, err
		}
		domain, err := repositoryDomain(ctx, daoReg, repo, orgId, domainName, rhDomainName, communityDomainName)
		if err != nil {
			return mapping, err
		}
		// The template's linked snapshot is authoritative even when last_snapshot_uuid is absent.
		repoOver, err := contentOverridesForRepo(domain, template.UUID, contentPath, repo, false)
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

func ContentOverridesForRepo(domainName string, templateUUID string, pulpContentPath string, repo api.RepositoryResponse) ([]caliri.ContentOverrideDTO, error) {
	return contentOverridesForRepo(domainName, templateUUID, pulpContentPath, repo, true)
}

func contentOverridesForRepo(domainName string, templateUUID string, pulpContentPath string, repo api.RepositoryResponse, requireLastSnapshot bool) ([]caliri.ContentOverrideDTO, error) {
	mapping := []caliri.ContentOverrideDTO{}
	if requireLastSnapshot && repo.LastSnapshot == nil { // ignore repos without a snapshot outside template content resolution
		return mapping, nil
	}

	isExtendedReleaseRepo := repo.OrgID == config.RedHatOrg && repo.ExtendedRelease != ""
	shouldOverrideURL := isExtendedReleaseRepo ||
		repo.Origin == config.OriginExternal ||
		repo.Origin == config.OriginCommunity ||
		repo.Origin == config.OriginUpload

	contentLabel := repo.Label
	if isExtendedReleaseRepo {
		contentLabel = normalizeExtendedReleaseLabel(repo.Label)
	}

	mapping = append(mapping, caliri.ContentOverrideDTO{
		Name:         utils.Ptr(candlepin_client.OverrideNameCaCert),
		ContentLabel: &contentLabel,
		Value:        utils.Ptr(" "), // use a single space because candlepin doesn't allow "" or null
	})
	// Disable OCSP checking, as aws doesn't support it?
	mapping = append(mapping, caliri.ContentOverrideDTO{
		Name:         utils.Ptr(candlepin_client.OverrideSSLVerifyStatus),
		ContentLabel: &contentLabel,
		Value:        utils.Ptr("0"),
	})

	if shouldOverrideURL {
		distPath, _, err := getDistPathAndName(repo, templateUUID)
		if err != nil {
			return mapping, err
		}

		path, err := url.JoinPath(pulpContentPath, domainName, distPath)
		if err != nil {
			return mapping, err
		}

		mapping = append(mapping, caliri.ContentOverrideDTO{
			Name:         utils.Ptr(candlepin_client.OverrideNameBaseUrl),
			ContentLabel: &contentLabel,
			Value:        &path,
		})
		if repo.ModuleHotfixes {
			mapping = append(mapping, caliri.ContentOverrideDTO{
				Name:         utils.Ptr(candlepin_client.OverrideModuleHotfixes),
				ContentLabel: &contentLabel,
				Value:        utils.Ptr("1"),
			})
		}
	}
	return mapping, nil
}

func customTemplateSnapshotPath(templateUUID string, repoUUID string) string {
	return fmt.Sprintf("templates/%v/%v", templateUUID, repoUUID)
}
