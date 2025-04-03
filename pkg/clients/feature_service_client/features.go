package feature_service_client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	caliri "github.com/content-services/caliri/release/v4"
	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/cache"
	"github.com/content-services/content-sources-backend/pkg/config"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/rs/zerolog/log"
)

type FeaturesResponse struct {
	Features []Feature `json:"features"`
}

type Feature struct {
	Name  string `json:"name"`
	Rules Rules  `json:"rules"`
}

type Rules struct {
	MatchProducts []MatchProducts `json:"matchProducts"`
}

type MatchProducts struct {
	EngIDs []int `json:"engIds"`
}

type FeatureStatusResponse struct {
	Features []struct {
		Name     string `json:"name"`
		Entitled bool   `json:"isEntitled"`
	} `json:"features"`
}

func (fs featureServiceImpl) ListFeatures(ctx context.Context) (FeaturesResponse, int, error) {
	statusCode := http.StatusInternalServerError
	var err error

	server := config.Get().Clients.FeatureService.Server
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server, nil)
	if err != nil {
		return FeaturesResponse{}, statusCode, err
	}

	var body []byte
	resp, err := fs.client.Do(req)
	if resp != nil {
		defer resp.Body.Close()

		body, err = io.ReadAll(resp.Body)
		if err != nil {
			return FeaturesResponse{}, statusCode, fmt.Errorf("error during read response body: %w", err)
		}

		if resp.StatusCode != 0 {
			statusCode = resp.StatusCode
		}
	}
	if err != nil {
		return FeaturesResponse{}, statusCode, fmt.Errorf("error during GET request: %w", err)
	}
	if statusCode < 200 || statusCode >= 300 {
		return FeaturesResponse{}, statusCode, fmt.Errorf("unexpected status code with body: %s", string(body))
	}

	var featResp FeaturesResponse
	err = json.Unmarshal(body, &featResp)
	if err != nil {
		return FeaturesResponse{}, statusCode, fmt.Errorf("error during unmarshal response body: %w", err)
	}

	return featResp, statusCode, nil
}

func (fs featureServiceImpl) GetFeatureStatusByOrgID(ctx context.Context, orgID string) (api.FeatureStatus, int, error) {
	statusCode := http.StatusInternalServerError
	var err error

	features := config.Get().Options.FeatureFilter
	featureParams := make([]string, len(features))
	for i, feature := range features {
		featureParams[i] = fmt.Sprintf("features=%s", url.QueryEscape(feature))
	}
	path := fmt.Sprintf("/featureStatus?accountId=%s", orgID)
	fullPath := path + "&" + strings.Join(featureParams, "&")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, config.Get().Clients.FeatureService.Server+fullPath, nil)
	if err != nil {
		return api.FeatureStatus{}, 0, err
	}

	var body []byte
	resp, err := fs.client.Do(req)
	if resp != nil {
		defer resp.Body.Close()

		if resp.StatusCode != 0 {
			statusCode = resp.StatusCode
		}

		body, err = io.ReadAll(resp.Body)
		if err != nil {
			return api.FeatureStatus{}, http.StatusInternalServerError, fmt.Errorf("error during read response body: %w", err)
		}
	}
	if err != nil {
		return api.FeatureStatus{}, statusCode, fmt.Errorf("error during GET request: %w", err)
	}
	if statusCode < 200 || statusCode >= 300 {
		return api.FeatureStatus{}, statusCode, fmt.Errorf("unexpected status code with body: %s", string(body))
	}

	var featStatus FeatureStatusResponse
	err = json.Unmarshal(body, &featStatus)
	if err != nil {
		return api.FeatureStatus{}, statusCode, fmt.Errorf("error during unmarshal response body: %w", err)
	}

	var entitledFeatures []string
	for _, feature := range featStatus.Features {
		if feature.Entitled {
			entitledFeatures = append(entitledFeatures, feature.Name)
		}
	}

	featStatusResp := api.FeatureStatus{
		OrgID:       orgID,
		FeatureList: entitledFeatures,
	}

	return featStatusResp, statusCode, nil
}

func (fs featureServiceImpl) GetEntitledFeatures(ctx context.Context, orgID string) ([]string, error) {
	entitledFeatures := []string{"RHEL-OS-x86_64", "RHEL-OS-aarch64"}

	if config.Get().Clients.FeatureService.Server == "" || orgID == config.RedHatOrg {
		if config.Get().Options.EntitleAll {
			return config.Get().Options.FeatureFilter, nil
		}
		return entitledFeatures, nil
	}

	cacheHit, err := fs.cache.GetFeatureStatus(ctx)
	if err != nil && !errors.Is(err, cache.NotFound) {
		log.Logger.Error().Err(err).Msg("featureStatus: error reading from cache")
	}
	if cacheHit != nil {
		entitledFeatures = append(entitledFeatures, cacheHit.FeatureList...)
		return entitledFeatures, nil
	}

	features, statusCode, err := fs.GetFeatureStatusByOrgID(ctx, orgID)
	if err != nil {
		return []string{}, ce.NewErrorResponse(statusCode, "error checking feature status", err.Error())
	}
	entitledFeatures = append(entitledFeatures, features.FeatureList...)

	err = fs.cache.SetFeatureStatus(ctx, features)
	if err != nil {
		log.Logger.Error().Err(err).Msg("featureStatus: error writing to cache")
	}

	return entitledFeatures, nil
}

const cdnServer = "https://cdn.redhat.com"

func ProductToRepoJSON(product *caliri.ProductDTO, featureName string) []api.FeatureServiceContentResponse {
	if product == nil {
		return []api.FeatureServiceContentResponse{}
	}

	var content []api.FeatureServiceContentResponse
	productContent := product.GetProductContent()
	for _, pc := range productContent {
		c := parseProductContent(&pc, featureName)

		if strings.Contains(c.Name, "Source RPMs") ||
			strings.Contains(c.Name, "Debug RPMs") ||
			strings.Contains(c.Name, "ISOs") ||
			strings.Contains(c.Name, "Source ISOs") ||
			strings.Contains(c.Name, "Files") {
			continue
		}

		content = append(content, parseProductContent(&pc, featureName))
	}
	return content
}

func parseProductContent(productContent *caliri.ProductContentDTO, featureName string) api.FeatureServiceContentResponse {
	if productContent == nil {
		return api.FeatureServiceContentResponse{}
	}

	var content api.FeatureServiceContentResponse
	contentDTO := productContent.GetContent()

	content.Name = contentDTO.GetName()
	content.URL = contentDTO.GetContentUrl()
	content.RedHatRepoStructure.Name = contentDTO.GetName()
	content.RedHatRepoStructure.ContentLabel = contentDTO.GetLabel()
	content.RedHatRepoStructure.Arch = getArchFromArches(contentDTO.GetArches())
	content.RedHatRepoStructure.DistributionVersion = getVersionFromLabel(contentDTO.GetLabel())
	content.RedHatRepoStructure.URL = getURLFromContentURL(contentDTO.GetContentUrl(), getVersionFromLabel(contentDTO.GetLabel()), getArchFromArches(contentDTO.GetArches()))
	content.RedHatRepoStructure.FeatureName = featureName

	return content
}

func getURLFromContentURL(contentURL string, version string, arch string) string {
	url := cdnServer + contentURL
	if version != "" {
		url = strings.Replace(url, "$releasever", version, -1)
	}
	if arch != "" {
		url = strings.Replace(url, "$basearch", arch, -1)
	}
	return url
}

func getVersionFromLabel(label string) string {
	splitLabel := strings.Split(label, "-")
	var version string
	for i := 0; i < len(splitLabel); i++ {
		if splitLabel[i] == "rhel" {
			version = splitLabel[i+1]

			// high-availability repos have different label formatting "rhel-ha-for-rhel-<version>..."
			if version == "ha" {
				continue
			}

			break
		}
	}
	return version
}

func getArchFromArches(arches string) string {
	if strings.Contains(arches, "x86_64") {
		return "x86_64"
	}
	if strings.Contains(arches, "aarch64") {
		return "aarch64"
	}
	return arches
}
