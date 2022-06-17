package external_repos

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
)

// IBUrlsFromDir Import Repo Urls from image builder's git repo
// The dirPath should point to GITDIR/distributions/ which contains
// 		the repo directories.  Each directory contains a json file
//		with the same name as the directory itself:
//		./distributions/rhel-90/rhel-90.json
func IBUrlsFromDir(dirPath string) ([]string, error) {
	var urls []string
	files, err := scanDir(dirPath)
	if err != nil {
		return nil, err
	}
	for i := 0; i < len(files); i++ {
		subUrls, err := extractUrls(files[i])
		if err != nil {
			return nil, err
		}
		urls = append(urls, subUrls...)
	}
	return urls, nil
}

type ImageBuilderRepoJson struct {
	MainArch   ImageBuilderArch `json:"x86_64"`
	PlatformId string           `json:"module_platform_id"`
}

type ImageBuilderArch struct {
	Repositories []ImageBuilderRepo `json:"repositories"`
	ImageTypes   []string           `json:"image_types"`
}

type ImageBuilderRepo struct {
	BaseUrl string `json:"baseurl"`
}

func extractUrls(filePath string) ([]string, error) {
	var repoUrls []string
	jsonStr, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	parsed := ImageBuilderRepoJson{}
	err = json.Unmarshal(jsonStr, &parsed)
	if err != nil {
		return nil, err
	}
	repos := parsed.MainArch.Repositories
	for i := 0; i < len(repos); i++ {
		repoUrls = append(repoUrls, repos[i].BaseUrl)
	}
	return repoUrls, nil
}

// Scans the IB Distributions directory for repo json files
//   This assumes that some directory "distributions/foo/" contains
// 	 a json file  distributions/foo/foo.json
func scanDir(dirPath string) ([]string, error) {
	var foundFiles []string

	files, err := ioutil.ReadDir(dirPath)
	if err != nil {
		return nil, err
	}

	for i := 0; i < len(files); i++ {
		if files[i].IsDir() && files[i].Name()[0:1] != "." {
			filename := fmt.Sprintf("%s.json", files[i].Name())
			foundFiles = append(foundFiles, filepath.Join(dirPath, files[i].Name(), filename))
		}
	}

	return foundFiles, nil
}
