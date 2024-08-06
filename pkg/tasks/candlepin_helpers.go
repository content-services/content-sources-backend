package tasks

import (
	"fmt"
	"net/url"

	caliri "github.com/content-services/caliri/release/v4"
	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/candlepin_client"
	"github.com/content-services/content-sources-backend/pkg/config"
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
	return caliri.ContentDTO{
		Id:         &id,
		Type:       &repoType,
		Label:      &repoLabel,
		Name:       &repoName,
		Vendor:     &repoVendor,
		GpgUrl:     gpgKeyUrl,
		ContentUrl: &repo.URL, // Set to upstream URL, but it is not used. Will use content overrides instead.
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
