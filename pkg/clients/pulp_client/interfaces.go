package pulp_client

import (
	"context"
	"os"

	zest "github.com/content-services/zest/release/v2024"
)

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

	// Livez
	Livez(ctx context.Context) error
}

type PulpClient interface {
	// Artifacts
	LookupArtifact(ctx context.Context, sha256sum string) (*string, error)

	// Remotes
	CreateRpmRemote(ctx context.Context, name string, url string, clientCert *string, clientKey *string, caCert *string) (*zest.RpmRpmRemoteResponse, error)
	UpdateRpmRemote(ctx context.Context, pulpHref string, url string, clientCert *string, clientKey *string, caCert *string) (string, error)
	GetRpmRemoteByName(ctx context.Context, name string) (*zest.RpmRpmRemoteResponse, error)
	GetRpmRemoteList(ctx context.Context) ([]zest.RpmRpmRemoteResponse, error)
	DeleteRpmRemote(ctx context.Context, pulpHref string) (string, error)

	// Content Guards
	CreateOrUpdateGuardsForOrg(ctx context.Context, orgId string) (string, error)
	CreateOrUpdateFeatureGuard(ctx context.Context, featureName string) (string, error)

	// Tasks
	GetTask(ctx context.Context, taskHref string) (zest.TaskResponse, error)
	PollTask(ctx context.Context, taskHref string) (*zest.TaskResponse, error)
	CancelTask(ctx context.Context, taskHref string) (zest.TaskResponse, error)
	GetContentPath(ctx context.Context) (string, error)

	// Package
	CreatePackage(ctx context.Context, artifactHref *string, uploadHref *string) (string, error)
	LookupPackage(ctx context.Context, sha256sum string) (*string, error)
	ListVersionAllPackages(ctx context.Context, versionHref string) (pkgs []zest.RpmPackageResponse, err error)

	// Rpm Repository
	CreateRpmRepository(ctx context.Context, uuid string, rpmRemotePulpRef *string) (*zest.RpmRpmRepositoryResponse, error)
	GetRpmRepositoryByName(ctx context.Context, name string) (*zest.RpmRpmRepositoryResponse, error)
	GetRpmRepositoryByRemote(ctx context.Context, pulpHref string) (*zest.RpmRpmRepositoryResponse, error)
	SyncRpmRepository(ctx context.Context, rpmRpmRepositoryHref string, remoteHref *string) (string, error)
	DeleteRpmRepository(ctx context.Context, rpmRepositoryHref string) (string, error)

	// Rpm Repository Version
	GetRpmRepositoryVersion(ctx context.Context, href string) (*zest.RepositoryVersionResponse, error)
	DeleteRpmRepositoryVersion(ctx context.Context, href string) (*string, error)
	RepairRpmRepositoryVersion(ctx context.Context, href string) (string, error)
	ModifyRpmRepositoryContent(ctx context.Context, repoHref string, contentHrefsToAdd []string, contentHrefsToRemove []string) (string, error)

	// RpmPublication
	CreateRpmPublication(ctx context.Context, versionHref string) (*string, error)
	FindRpmPublicationByVersion(ctx context.Context, versionHref string) (*zest.RpmRpmPublicationResponse, error)

	// Distribution
	CreateRpmDistribution(ctx context.Context, publicationHref string, name string, basePath string, contentGuardHref *string) (*string, error)
	FindDistributionByPath(ctx context.Context, path string) (*zest.RpmRpmDistributionResponse, error)
	DeleteRpmDistribution(ctx context.Context, rpmDistributionHref string) (*string, error)
	UpdateRpmDistribution(ctx context.Context, rpmDistributionHref string, rpmPublicationHref string, distributionName string, basePath string, contentGuardHref *string) (string, error)

	// Domains
	LookupOrCreateDomain(ctx context.Context, name string) (string, error)
	LookupDomain(ctx context.Context, name string) (string, error)
	UpdateDomainIfNeeded(ctx context.Context, name string) error

	// Status
	Status(ctx context.Context) (*zest.StatusResponse, error)

	// Livez
	Livez(ctx context.Context) error

	// Orphans
	OrphanCleanup(ctx context.Context) (string, error)

	// Chainable
	WithDomain(domainName string) PulpClient

	// Uploads
	CreateUpload(ctx context.Context, size int64) (*zest.UploadResponse, int, error)
	UploadChunk(ctx context.Context, uploadHref string, contentRange string, file *os.File, sha256 string) (*zest.UploadResponse, int, error)
	FinishUpload(ctx context.Context, uploadHref string, sha256 string) (*zest.AsyncOperationResponse, int, error)
}
