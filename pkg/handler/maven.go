package handler

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/content-services/content-sources-backend/pkg/utils"
)

const mavenCentralHTTPTimeout = 30 * time.Second

var mavenCentralBaseURL = "https://repo.maven.apache.org/maven2"

var lightwellVersionSuffix = regexp.MustCompile(`\.rhlw-.*$`)

type pomXML struct {
	XMLName      xml.Name `xml:"project"`
	Description  string   `xml:"description"`
	URL          string   `xml:"url"`
	Organization struct {
		Name string `xml:"name"`
	} `xml:"organization"`
	Parent struct {
		GroupID    string `xml:"groupId"`
		ArtifactID string `xml:"artifactId"`
		Version    string `xml:"version"`
	} `xml:"parent"`
	Licenses struct {
		License []struct {
			Name string `xml:"name"`
		} `xml:"license"`
	} `xml:"licenses"`
}

type mavenPomMetadata struct {
	Summary    *string
	License    *string
	ProjectURL *string
	Author     *string
}

func mavenGroupPath(groupID string) string {
	return strings.ReplaceAll(groupID, ".", "/")
}

func mavenCentralPomPath(groupID, artifactID, version string) string {
	groupPath := mavenGroupPath(groupID)
	return fmt.Sprintf("/%s/%s/%s/%s-%s.pom", groupPath, artifactID, version, artifactID, version)
}

func mavenCentralPomURL(groupID, artifactID, version string) string {
	return mavenCentralBaseURL + mavenCentralPomPath(groupID, artifactID, version)
}

func stripLightwellVersionSuffix(version string) string {
	return lightwellVersionSuffix.ReplaceAllString(version, "")
}

func isValid(value string) bool {
	return value != "" &&
		!strings.Contains(value, "..") &&
		!strings.ContainsAny(value, `/\@:`)
}

func fetchMavenPom(ctx context.Context, httpClient *http.Client, groupID, artifactID, version string) ([]byte, error) {
	if !isValid(groupID) || !isValid(artifactID) || !isValid(version) {
		return nil, fmt.Errorf("invalid maven coordinates")
	}

	if httpClient == nil {
		httpClient = &http.Client{Timeout: mavenCentralHTTPTimeout}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, mavenCentralBaseURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create POM request: %w", err)
	}
	req.URL.Path = strings.TrimSuffix(req.URL.Path, "/") + mavenCentralPomPath(groupID, artifactID, version)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch POM: %w", err)
	}
	defer resp.Body.Close()

	pomURL := req.URL.String()
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("POM not found: %s", pomURL)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d fetching POM: %s", resp.StatusCode, pomURL)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read POM body: %w", err)
	}

	return data, nil
}

func parsePom(data []byte) (mavenPomMetadata, error) {
	var pom pomXML
	if err := xml.Unmarshal(data, &pom); err != nil {
		return mavenPomMetadata{}, fmt.Errorf("failed to parse POM XML: %w", err)
	}

	metadata := mavenPomMetadata{}

	description := strings.TrimSpace(pom.Description)
	if description != "" {
		metadata.Summary = utils.Ptr(description)
	}

	projectURL := strings.TrimSpace(pom.URL)
	if projectURL != "" {
		metadata.ProjectURL = utils.Ptr(projectURL)
	}

	for _, lic := range pom.Licenses.License {
		name := strings.TrimSpace(lic.Name)
		if name != "" {
			metadata.License = utils.Ptr(name)
			break
		}
	}

	author := strings.TrimSpace(pom.Organization.Name)
	if author != "" {
		metadata.Author = utils.Ptr(author)
	}

	return metadata, nil
}

func fetchMavenCentralMetadata(ctx context.Context, httpClient *http.Client, groupID, artifactID, version string) (mavenPomMetadata, error) {
	upstreamVersion := stripLightwellVersionSuffix(version)

	data, err := fetchMavenPom(ctx, httpClient, groupID, artifactID, upstreamVersion)
	if err != nil {
		return mavenPomMetadata{}, err
	}

	metadata, err := parsePom(data)
	if err != nil {
		return mavenPomMetadata{}, err
	}

	if metadata.License == nil || metadata.Author == nil {
		var pom pomXML
		if xml.Unmarshal(data, &pom) == nil && pom.Parent.GroupID != "" && pom.Parent.ArtifactID != "" && pom.Parent.Version != "" {
			parentData, parentErr := fetchMavenPom(ctx, httpClient, pom.Parent.GroupID, pom.Parent.ArtifactID, pom.Parent.Version)
			if parentErr == nil {
				parentMetadata, parseErr := parsePom(parentData)
				if parseErr == nil {
					if metadata.License == nil {
						metadata.License = parentMetadata.License
					}
					if metadata.Author == nil {
						metadata.Author = parentMetadata.Author
					}
				}
			}
		}
	}

	return metadata, nil
}

func isMavenCentralPomNotFound(err error) bool {
	return err != nil && strings.Contains(err.Error(), "POM not found")
}
