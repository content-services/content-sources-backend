package external_repos

import (
	"embed"
	"encoding/json"
	"os"
)

const Filename = "./pkg/external_repos/external_repos.json"

//go:embed "external_repos.json"
//go:embed "ca.pem"

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

// LoadCA Load the CA certificate from the embedded file at
// 'test_files/ca.pem'
// Return []byte with the content of the file and nil for error
// if the process finish with success, else it returns nil and
// the error is filled.
func LoadCA() ([]byte, error) {
	var (
		caCert []byte
		err    error
	)
	if caCert, err = fs.ReadFile("ca.pem"); err != nil {
		return nil, err
	}
	return caCert, nil
}

// LoadFruitFromFile Loads repo urls from the external file
func LoadFruitFromFile() ([]string, error) {
	var (
		fruits   []string
		contents []byte
		err      error
	)

	contents, err = os.ReadFile("./pkg/external_repos/fruit.json")
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(contents, &fruits)
	if err != nil {
		return nil, err
	}
	return fruits, nil
}
