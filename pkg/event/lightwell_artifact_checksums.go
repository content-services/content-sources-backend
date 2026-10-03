package event

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var validSHA256 = regexp.MustCompile(`^[0-9a-f]{64}$`)

// ArtifactChecksumRequest identifies a single artifact whose SHA-256 sidecar
// should be fetched from the Pulp content server.
type ArtifactChecksumRequest struct {
	PackageName string // Maven: "org.example:lib-a", Python: "requests"
	Version     string // e.g. "1.0.1.rhlw-00001"
}

// FetchArtifactChecksums fetches .sha256 sidecar files from the Pulp content
// server for each request. Returns a map: version → (filename → sha256).
// Errors for individual artifacts are logged as warnings and omitted from the
// result. Only Java (Maven) repositories are supported; Python repositories
// log a warning and return an empty map.
func FetchArtifactChecksums(
	ctx context.Context,
	httpClient *http.Client,
	contentBaseURL string,
	repoType string,
	items []ArtifactChecksumRequest,
	authUser, authPassword string,
) map[string]map[string]string {
	logger := log.With().Str("content_base", contentBaseURL).Logger()

	if repoType != "maven" {
		logger.Warn().Str("repo_type", repoType).
			Msg("artifact checksum fetch not supported for this ecosystem")
		return nil
	}

	result := make(map[string]map[string]string, len(items))
	seen := make(map[string]struct{})
	for _, item := range items {
		if _, ok := seen[item.Version]; ok {
			continue
		}
		seen[item.Version] = struct{}{}

		filename, sidecarURL, err := mavenSidecarURL(contentBaseURL, item.PackageName, item.Version)
		if err != nil {
			logger.Warn().Err(err).
				Str("package", item.PackageName).
				Str("version", item.Version).
				Msg("cannot construct sidecar URL")
			continue
		}

		sha, err := fetchSidecar(ctx, httpClient, sidecarURL, authUser, authPassword, logger)
		if err != nil {
			logger.Warn().Err(err).
				Str("package", item.PackageName).
				Str("version", item.Version).
				Str("url", sidecarURL).
				Msg("failed to fetch artifact checksum")
			continue
		}

		result[item.Version] = map[string]string{filename: sha}
	}

	return result
}

// mavenSidecarURL constructs the .sha256 sidecar URL for a Maven JAR artifact.
// PackageName must be in "group:artifact" format.
func mavenSidecarURL(contentBaseURL, packageName, version string) (filename, url string, err error) {
	parts := strings.SplitN(packageName, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid Maven package name %q: expected group:artifact", packageName)
	}
	groupID := parts[0]
	artifactID := parts[1]

	groupPath := strings.ReplaceAll(groupID, ".", "/")
	jarName := artifactID + "-" + version + ".jar"
	sidecarPath := fmt.Sprintf("%s/%s/%s/%s/%s.sha256",
		strings.TrimRight(contentBaseURL, "/"),
		groupPath, artifactID, version, jarName)

	return jarName, sidecarPath, nil
}

const maxSidecarBytes = 256

func fetchSidecar(
	ctx context.Context,
	client *http.Client,
	url, authUser, authPassword string,
	logger zerolog.Logger,
) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("error creating request: %w", err)
	}

	if authUser != "" {
		req.SetBasicAuth(authUser, authPassword)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxSidecarBytes))
	if err != nil {
		return "", fmt.Errorf("error reading response: %w", err)
	}

	sha := strings.TrimSpace(string(body))
	if !validSHA256.MatchString(sha) {
		return "", fmt.Errorf("invalid SHA-256 format: %q", sha)
	}

	return sha, nil
}
