package candlepin_client

import (
	"context"

	caliri "github.com/content-services/caliri/release/v4"
)

func GetProductID(ownerKey string) string {
	return "product-" + ownerKey
}

func (c *cpClientImpl) FetchProduct(ctx context.Context, orgID string) (*caliri.ProductDTO, error) {
	ctx, client, err := getCandlepinClient(ctx)
	if err != nil {
		return nil, err
	}

	found, httpResp, err := client.OwnerProductAPI.GetProductById(ctx, OwnerKey(orgID), GetProductID(OwnerKey(orgID))).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if httpResp != nil && httpResp.StatusCode == 404 {
		return nil, nil
	}
	if err != nil {
		return nil, errorWithResponseBody("couldn't fetch product", httpResp, err)
	}

	return found, nil
}

func (c *cpClientImpl) CreateProduct(ctx context.Context, orgID string) error {
	ctx, client, err := getCandlepinClient(ctx)
	if err != nil {
		return err
	}

	productID := GetProductID(OwnerKey(orgID))
	found, err := c.FetchProduct(ctx, OwnerKey(orgID))
	if found != nil || err != nil {
		return err
	}
	_, httpResp, err := client.OwnerProductAPI.CreateProduct(ctx, OwnerKey(orgID)).ProductDTO(caliri.ProductDTO{Name: &productID, Id: &productID}).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return errorWithResponseBody("couldn't create product", httpResp, err)
	}
	return nil
}

func (c *cpClientImpl) AddContentBatchToProduct(ctx context.Context, orgID string, contentIDs []string) error {
	ctx, client, err := getCandlepinClient(ctx)
	if err != nil {
		return err
	}

	productID := GetProductID(OwnerKey(orgID))

	contentMap := make(map[string]bool)
	for _, id := range contentIDs {
		contentMap[id] = false
	}
	_, httpResp, err := client.OwnerProductAPI.AddContentsToProduct(ctx, OwnerKey(orgID), productID).RequestBody(contentMap).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return errorWithResponseBody("couldn't add contents to product", httpResp, err)
	}
	return nil
}
