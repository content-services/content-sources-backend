package candlepin_client

import (
	"context"
	"fmt"
	"strings"

	caliri "github.com/content-services/caliri/release/v4"
)

func GetContentID(repoConfigUUID string) string {
	return strings.Replace(repoConfigUUID, "-", "", -1)
}

func (c *cpClientImpl) ListContents(ctx context.Context, ownerKey string) ([]string, error) {
	ctx, client, err := getCandlepinClient(ctx)
	if err != nil {
		return nil, err
	}

	labels := []string{}
	contents, httpResp, err := client.OwnerContentAPI.GetContentsByOwner(ctx, ownerKey).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return labels, fmt.Errorf("could not fetch contents for owner %w", err)
	}

	for _, c := range contents {
		labels = append(labels, *c.Label)
	}
	return labels, nil
}

func (c *cpClientImpl) CreateContentBatch(ctx context.Context, ownerKey string, content []caliri.ContentDTO) error {
	ctx, client, err := getCandlepinClient(ctx)
	if err != nil {
		return err
	}

	// required contentDTO params: id, name, label, type, vendor
	_, httpResp, err := client.OwnerContentAPI.CreateContentBatch(ctx, ownerKey).ContentDTO(content).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return errorWithResponseBody("couldn't create content batch", httpResp, err)
	}
	return nil
}

func (c *cpClientImpl) CreateContent(ctx context.Context, ownerKey string, content caliri.ContentDTO) error {
	ctx, client, err := getCandlepinClient(ctx)
	if err != nil {
		return err
	}
	// required contentDTO params: id, name, label, type, vendor
	_, httpResp, err := client.OwnerContentAPI.CreateContent(ctx, ownerKey).ContentDTO(content).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return errorWithResponseBody("couldn't create content batch", httpResp, err)
	}
	return nil
}
