package candlepin_client

import (
	"context"
	"net/http"

	caliri "github.com/content-services/caliri/release/v4"
)

func GetProductID(ownerKey string) string {
	return "product-" + ownerKey
}

func (c *cpClientImpl) FetchProduct(ctx context.Context, orgID string, productID string) (*caliri.ProductDTO, error) {
	ctx, client, err := getCandlepinClient(ctx)
	if err != nil {
		return nil, err
	}

	found, httpResp, err := client.OwnerProductAPI.GetProductById(ctx, OwnerKey(orgID), productID).Execute()
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
	found, err := c.FetchProduct(ctx, OwnerKey(orgID), productID)
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

func (c *cpClientImpl) ListProducts(ctx context.Context, orgID string, productIDs []string) ([]caliri.ProductDTO, error) {
	ctx, client, err := getCandlepinClient(ctx)
	if err != nil {
		return []caliri.ProductDTO{}, nil
	}

	// we want to set custom to "exclusive" so that the results are filtered to the org
	// but this also excludes content imported from manifests, which is how we access content locally
	var custom string
	if OwnerKey(orgID) == DevelOrgKey {
		custom = ""
	} else {
		custom = "exclusive"
	}

	products, httpResp, err := client.OwnerProductAPI.
		GetProductsByOwner(ctx, OwnerKey(orgID)).
		Product(productIDs).
		Custom(custom).
		Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
		return []caliri.ProductDTO{}, nil
	}
	if err != nil {
		return []caliri.ProductDTO{}, errorWithResponseBody("couldn't list products", httpResp, err)
	}

	return products, nil
}
