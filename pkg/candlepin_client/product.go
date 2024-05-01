package candlepin_client

import (
	"context"

	caliri "github.com/content-services/caliri/release/v4"
)

func GetProductID(ownerKey string) string {
	return "product-" + ownerKey
}

func (c *cpClientImpl) FetchProduct(ctx context.Context, ownerKey string, productID string) (*caliri.ProductDTO, error) {
	ctx, client, err := getCandlepinClient(ctx)
	if err != nil {
		return nil, err
	}

	found, httpResp, err := client.OwnerProductAPI.GetProductById(ctx, ownerKey, productID).Execute()
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

func (c *cpClientImpl) CreateProduct(ctx context.Context, ownerKey string) error {
	ctx, client, err := getCandlepinClient(ctx)
	if err != nil {
		return err
	}

	productID := GetProductID(ownerKey)
	found, err := c.FetchProduct(ctx, ownerKey, productID)
	if found != nil || err != nil {
		return err
	}
	_, httpResp, err := client.OwnerProductAPI.CreateProduct(ctx, ownerKey).ProductDTO(caliri.ProductDTO{Name: &productID, Id: &productID}).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return errorWithResponseBody("couldn't create product", httpResp, err)
	}
	return nil
}

func (c *cpClientImpl) AddContentBatchToProduct(ctx context.Context, ownerKey string, contentIDs []string) error {
	ctx, client, err := getCandlepinClient(ctx)
	if err != nil {
		return err
	}

	productID := GetProductID(ownerKey)

	contentMap := make(map[string]bool)
	for _, id := range contentIDs {
		contentMap[id] = true
	}
	_, httpResp, err := client.OwnerProductAPI.AddContentsToProduct(ctx, ownerKey, productID).RequestBody(contentMap).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return errorWithResponseBody("couldn't add contents to product", httpResp, err)
	}
	return nil
}
