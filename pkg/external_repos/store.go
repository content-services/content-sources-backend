package external_repos

import (
	"embed"
	"encoding/json"
	"os"
)

const Filename = "./pkg/external_repos/external_repos.json"

//go:embed "external_repos.json"

var fs embed.FS

type ExternalRepository struct {
	BaseUrl string `json:"base_url"`
}

// SaveToFile Saves a list of repo urls to the external file
func SaveToFile(repoUrls []string) error {
	var (
		repos    []ExternalRepository
		err      error
		repoJson []byte
	)
	for i := 0; i < len(repoUrls); i++ {
		repos = append(repos, ExternalRepository{BaseUrl: repoUrls[i]})
	}
	repoJson, err = json.MarshalIndent(&repos, "", "    ")
	if err != nil {
		return err
	}
	err = os.WriteFile(Filename, repoJson, 0644)
	if err != nil {
		return err
	}
	return nil
}

// LoadFromFile Loads repo urls from the external file
func LoadFromFile() ([]ExternalRepository, error) {
	var (
		repos    []ExternalRepository
		contents []byte
		err      error
	)

	contents, err = fs.ReadFile("external_repos.json")
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(contents, &repos)
	if err != nil {
		return nil, err
	}
	return repos, nil
}

func GetBaseURLs(repos []ExternalRepository) []string {
	var urls []string
	for i := 0; i < len(repos); i++ {
		urls = append(urls, repos[i].BaseUrl)
	}
	return urls
}
