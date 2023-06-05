package pulp_client

import zest "github.com/content-services/zest/release/v3"

//go:generate mockery  --name pulp_client
type PulpClient interface {
	// Remotes
	CreateRpmRemote(name string, url string) (*zest.RpmRpmRemoteResponse, error)
	UpdateRpmRemoteUrl(pulpHref string, url string) (string, error)
	GetRpmRemoteByName(name string) (*zest.RpmRpmRemoteResponse, error)
	GetRpmRemoteList() ([]zest.RpmRpmRemoteResponse, error)
	DeleteRpmRemote(pulpHref string) (string, error)

	// Tasks
	GetTask(taskHref string) (zest.TaskResponse, error)
	PollTask(taskHref string) (*zest.TaskResponse, error)

	// Rpm Repository
	CreateRpmRepository(uuid string, rpmRemotePulpRef *string) (*zest.RpmRpmRepositoryResponse, error)
	GetRpmRepositoryByName(name string) (*zest.RpmRpmRepositoryResponse, error)
	GetRpmRepositoryByRemote(pulpHref string) (*zest.RpmRpmRepositoryResponse, error)
	SyncRpmRepository(rpmRpmRepositoryHref string, remoteHref *string) (string, error)

	// Rpm Repository Version
	GetRpmRepositoryVersion(href string) (*zest.RepositoryVersionResponse, error)

	// RpmPublication
	CreateRpmPublication(versionHref string) (*string, error)
	FindRpmPublicationByVersion(versionHref string) (*zest.RpmRpmPublicationResponse, error)

	// Distribution
	CreateRpmDistribution(publicationHref string, name string, basePath string) (*string, error)
	FindDistributionByPath(path string) (*zest.RpmRpmDistributionResponse, error)
}
