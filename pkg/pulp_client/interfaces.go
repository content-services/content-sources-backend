package pulp_client

import (
	"context"

	zest "github.com/content-services/zest/release/v2024"
)

//go:generate mockery  --name PulpGlobalClient --filename pulp_global_client_mock.go --inpackage
type PulpGlobalClient interface {
	// Domains
	LookupOrCreateDomain(ctx context.Context, name string) (string, error)
	LookupDomain(ctx context.Context, name string) (string, error)
	UpdateDomainIfNeeded(ctx context.Context, name string) error

	// Tasks
	GetTask(ctx context.Context, taskHref string) (zest.TaskResponse, error)
	PollTask(ctx context.Context, taskHref string) (*zest.TaskResponse, error)
	CancelTask(ctx context.Context, taskHref string) (zest.TaskResponse, error)
	GetContentPath(ctx context.Context) (string, error)
}

//go:generate mockery  --name PulpClient --filename pulp_client_mock.go --inpackage
type PulpClient interface {
	// Remotes
	CreateRpmRemote(ctx context.Context, name string, url string, clientCert *string, clientKey *string, caCert *string) (*zest.RpmRpmRemoteResponse, error)
	UpdateRpmRemote(ctx context.Context, pulpHref string, url string, clientCert *string, clientKey *string, caCert *string) (string, error)
	GetRpmRemoteByName(ctx context.Context, name string) (*zest.RpmRpmRemoteResponse, error)
	GetRpmRemoteList(ctx context.Context) ([]zest.RpmRpmRemoteResponse, error)
	DeleteRpmRemote(ctx context.Context, pulpHref string) (string, error)

	// Content Guards
	CreateOrUpdateGuardsForOrg(ctx context.Context, orgId string) (string, error)

	// Tasks
	GetTask(ctx context.Context, taskHref string) (zest.TaskResponse, error)
	PollTask(ctx context.Context, taskHref string) (*zest.TaskResponse, error)
	CancelTask(ctx context.Context, taskHref string) (zest.TaskResponse, error)
	GetContentPath(ctx context.Context) (string, error)

	// Rpm Repository
	CreateRpmRepository(ctx context.Context, uuid string, rpmRemotePulpRef *string) (*zest.RpmRpmRepositoryResponse, error)
	GetRpmRepositoryByName(ctx context.Context, name string) (*zest.RpmRpmRepositoryResponse, error)
	GetRpmRepositoryByRemote(ctx context.Context, pulpHref string) (*zest.RpmRpmRepositoryResponse, error)
	SyncRpmRepository(ctx context.Context, rpmRpmRepositoryHref string, remoteHref *string) (string, error)
	DeleteRpmRepository(ctx context.Context, rpmRepositoryHref string) (string, error)

	// Rpm Repository Version
	GetRpmRepositoryVersion(ctx context.Context, href string) (*zest.RepositoryVersionResponse, error)
	DeleteRpmRepositoryVersion(ctx context.Context, href string) (string, error)
	RepairRpmRepositoryVersion(ctx context.Context, href string) (string, error)

	// RpmPublication
	CreateRpmPublication(ctx context.Context, versionHref string) (*string, error)
	FindRpmPublicationByVersion(ctx context.Context, versionHref string) (*zest.RpmRpmPublicationResponse, error)

	// Distribution
	CreateRpmDistribution(ctx context.Context, publicationHref string, name string, basePath string, contentGuardHref *string) (*string, error)
	FindDistributionByPath(ctx context.Context, path string) (*zest.RpmRpmDistributionResponse, error)
	DeleteRpmDistribution(ctx context.Context, rpmDistributionHref string) (string, error)

	// Domains
	LookupOrCreateDomain(ctx context.Context, name string) (string, error)
	LookupDomain(ctx context.Context, name string) (string, error)
	UpdateDomainIfNeeded(ctx context.Context, name string) error

	// Status
	Status(ctx context.Context) (*zest.StatusResponse, error)

	// Orphans
	OrphanCleanup(ctx context.Context) (string, error)

	// Chainable
	WithDomain(domainName string) PulpClient
}
