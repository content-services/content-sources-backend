package candlepin_client

import (
	"context"
	"fmt"
	"strings"

	caliri "github.com/content-services/caliri/release/v4"
)

const OverrideNameBaseUrl = "baseurl"
const OverrideNameCaCert = "sslcacert"
const OverrideModuleHotfixes = "module_hotfixes"

func GetContentID(repoConfigUUID string) string {
	return strings.Replace(repoConfigUUID, "-", "", -1)
}

func (c *cpClientImpl) ListContents(ctx context.Context, orgID string) ([]string, []string, error) {
	ctx, client, err := getCandlepinClient(ctx)
	if err != nil {
		return nil, nil, err
	}

	labels := []string{}
	ids := []string{}
	contents, httpResp, err := client.OwnerContentAPI.GetContentsByOwner(ctx, OwnerKey(orgID)).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return labels, ids, fmt.Errorf("could not fetch contents for owner %w", err)
	}

	for _, c := range contents {
		labels = append(labels, *c.Label)
		ids = append(ids, *c.Id)
	}
	return labels, ids, nil
}

func (c *cpClientImpl) CreateContentBatch(ctx context.Context, orgID string, content []caliri.ContentDTO) error {
	ctx, client, err := getCandlepinClient(ctx)
	if err != nil {
		return err
	}

	// required contentDTO params: id, name, label, type, vendor
	_, httpResp, err := client.OwnerContentAPI.CreateContentBatch(ctx, OwnerKey(orgID)).ContentDTO(content).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return errorWithResponseBody("couldn't create content batch", httpResp, err)
	}
	return nil
}

func (c *cpClientImpl) CreateContent(ctx context.Context, orgID string, content caliri.ContentDTO) error {
	ctx, client, err := getCandlepinClient(ctx)
	if err != nil {
		return err
	}
	// required contentDTO params: id, name, label, type, vendor
	_, httpResp, err := client.OwnerContentAPI.CreateContent(ctx, OwnerKey(orgID)).ContentDTO(content).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return errorWithResponseBody("couldn't create content batch", httpResp, err)
	}
	return nil
}

func (c *cpClientImpl) UpdateContent(ctx context.Context, orgID string, repoConfigUUID string, content caliri.ContentDTO) error {
	ctx, client, err := getCandlepinClient(ctx)
	if err != nil {
		return err
	}
	_, httpResp, err := client.OwnerContentAPI.UpdateContent(ctx, OwnerKey(orgID), GetContentID(repoConfigUUID)).ContentDTO(content).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return errorWithResponseBody("couldn't update content", httpResp, err)
	}
	return nil
}

func (c *cpClientImpl) FetchContent(ctx context.Context, orgID string, repoConfigUUID string) (*caliri.ContentDTO, error) {
	ctx, client, err := getCandlepinClient(ctx)

	if err != nil {
		return nil, err
	}

	content, httpResp, err := client.OwnerContentAPI.GetContentById(ctx, OwnerKey(orgID), GetContentID(repoConfigUUID)).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		if httpResp != nil && httpResp.StatusCode == 404 {
			return nil, nil
		}
		return nil, errorWithResponseBody("couldn't fetch content", httpResp, err)
	}
	return content, nil
}

func (c *cpClientImpl) DeleteContent(ctx context.Context, ownerKey string, contentID string) error {
	ctx, client, err := getCandlepinClient(ctx)
	if err != nil {
		return err
	}
	httpResp, err := client.OwnerContentAPI.RemoveContent(ctx, ownerKey, contentID).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		if strings.Contains(err.Error(), "404") {
			return nil
		}
		return errorWithResponseBody("couldn't delete content", httpResp, err)
	}
	return nil
}
