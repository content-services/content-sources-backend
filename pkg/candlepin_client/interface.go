package candlepin_client

import (
	"context"

	caliri "github.com/content-services/caliri/release/v4"
)

//go:generate $GO_OUTPUT/mockery  --name CandlepinClient --filename candlepin_client_mock.go --inpackage
type CandlepinClient interface {
	CreateOwner(ctx context.Context) error
	ImportManifest(ctx context.Context, filename string) error

	// Products
	CreateProduct(ctx context.Context, orgID string) error
	FetchProduct(ctx context.Context, orgID string) (*caliri.ProductDTO, error)

	// Pools
	CreatePool(ctx context.Context, orgID string) (string, error)
	FetchPool(ctx context.Context, orgID string) (*caliri.PoolDTO, error)

	// Content
	ListContents(ctx context.Context, orgID string) ([]string, []string, error)
	CreateContentBatch(ctx context.Context, orgID string, content []caliri.ContentDTO) error
	CreateContent(ctx context.Context, orgID string, content caliri.ContentDTO) error
	AddContentBatchToProduct(ctx context.Context, orgID string, contentIDs []string) error
	UpdateContent(ctx context.Context, orgID string, repoConfigUUID string, content caliri.ContentDTO) error
	FetchContent(ctx context.Context, orgID string, repoConfigUUID string) (*caliri.ContentDTO, error)
	FetchContentsByLabel(ctx context.Context, orgID string, labels []string) ([]caliri.ContentDTO, error)
	DeleteContent(ctx context.Context, ownerKey string, repoConfigUUID string) error

	// Environments
	AssociateEnvironment(ctx context.Context, orgID string, templateName string, consumerUuid string) error
	CreateEnvironment(ctx context.Context, orgID string, name string, id string, prefix string) (*caliri.EnvironmentDTO, error)
	PromoteContentToEnvironment(ctx context.Context, templateUUID string, repoConfigUUIDs []string) error
	DemoteContentFromEnvironment(ctx context.Context, templateUUID string, repoConfigUUIDs []string) error
	FetchEnvironment(ctx context.Context, templateUUID string) (*caliri.EnvironmentDTO, error)
	UpdateContentOverrides(ctx context.Context, templateUUID string, dtos []caliri.ContentOverrideDTO) error
	FetchContentOverrides(ctx context.Context, templateUUID string) ([]caliri.ContentOverrideDTO, error)
	FetchContentOverridesForRepo(ctx context.Context, templateUUID string, label string) ([]caliri.ContentOverrideDTO, error)
	RemoveContentOverrides(ctx context.Context, templateUUID string, toRemove []caliri.ContentOverrideDTO) error
	DeleteEnvironment(ctx context.Context, templateUUID string) error
	RenameEnvironment(ctx context.Context, templateUUID, name string) (*caliri.EnvironmentDTO, error)
}
