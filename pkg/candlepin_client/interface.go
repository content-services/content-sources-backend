package candlepin_client

import (
	"context"

	caliri "github.com/content-services/caliri/release/v4"
)

//go:generate mockery  --name CandlepinClient --filename candlepin_client_mock.go --inpackage
type CandlepinClient interface {
	CreateOwner(ctx context.Context) error
	ImportManifest(ctx context.Context, filename string) error

	// Products
	CreateProduct(ctx context.Context, ownerKey string) error
	FetchProduct(ctx context.Context, ownerKey string, productID string) (*caliri.ProductDTO, error)

	// Pools
	CreatePool(ctx context.Context, ownerKey string) (string, error)
	FetchPool(ctx context.Context, ownerKey string, productID string) (*caliri.PoolDTO, error)

	// Content
	ListContents(ctx context.Context, ownerKey string) ([]string, []string, error)
	CreateContentBatch(ctx context.Context, ownerKey string, content []caliri.ContentDTO) error
	CreateContent(ctx context.Context, ownerKey string, content caliri.ContentDTO) error
	AddContentBatchToProduct(ctx context.Context, ownerKey string, contentIDs []string) error

	// Environments
	AssociateEnvironment(ctx context.Context, ownerKey string, templateName string, consumerUuid string) error
	CreateEnvironment(ctx context.Context, ownerKey string, name string, id string, prefix string) (*caliri.EnvironmentDTO, error)
	PromoteContentToEnvironment(ctx context.Context, envID string, contentIDs []string) error
	DemoteContentFromEnvironment(ctx context.Context, envID string, contentIDs []string) error
	FetchEnvironment(ctx context.Context, envID string) (*caliri.EnvironmentDTO, error)
}
