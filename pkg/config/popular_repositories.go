package config

import (
	"embed"
	"encoding/json"

	ce "github.com/content-services/content-sources-backend/pkg/errors"
)

//go:embed "popular_repositories.json"

var fs embed.FS
var PopularRepos []PopularRepository

// Should match api.PopularRepositoryResponse
type PopularRepository struct {
	UUID                 string   `json:"uuid"`                                // UUID of the repository if it exists for the user
	ExistingName         string   `json:"existing_name"`                       // Existing reference name for repository
	SuggestedName        string   `json:"suggested_name"`                      // Suggested name of the popular repository
	URL                  string   `json:"url"`                                 // URL of the remote yum repository
	DistributionVersions []string `json:"distribution_versions" example:"7,8"` // Versions to restrict client usage to
	DistributionArch     string   `json:"distribution_arch" example:"x86_64"`  // Architecture to restrict client usage to
	GpgKey               string   `json:"gpg_key"`                             // GPG key for repository
	MetadataVerification bool     `json:"metadata_verification"`               // Verify packages
}

func loadPopularRepos() error {
	jsonConfig, err := fs.ReadFile("popular_repositories.json")

	if err != nil {
		return ce.NewErrorResponseFromError("Could not read popular_repositories.json", err)
	}

	PopularRepos = []PopularRepository{}

	err = json.Unmarshal([]byte(jsonConfig), &PopularRepos)
	if err != nil {
		return ce.NewErrorResponseFromError("Could not read popular_repositories.json", err)
	}
	return nil
}
