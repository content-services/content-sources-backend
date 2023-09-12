package pulp_client

import zest "github.com/content-services/zest/release/v2023"

//go:generate mockery  --name PulpGlobalClient --filename pulp_global_client_mock.go --inpackage
type PulpGlobalClient interface {
	// Domains
	LookupOrCreateDomain(name string) (string, error)
	LookupDomain(name string) (string, error)
	UpdateDomainIfNeeded(name string) error

	// Tasks
	GetTask(taskHref string) (zest.TaskResponse, error)
	PollTask(taskHref string) (*zest.TaskResponse, error)
	CancelTask(taskHref string) (zest.TaskResponse, error)
}

//go:generate mockery  --name PulpClient --filename pulp_client_mock.go --inpackage
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
	CancelTask(taskHref string) (zest.TaskResponse, error)

	// Rpm Repository
	CreateRpmRepository(uuid string, rpmRemotePulpRef *string) (*zest.RpmRpmRepositoryResponse, error)
	GetRpmRepositoryByName(name string) (*zest.RpmRpmRepositoryResponse, error)
	GetRpmRepositoryByRemote(pulpHref string) (*zest.RpmRpmRepositoryResponse, error)
	SyncRpmRepository(rpmRpmRepositoryHref string, remoteHref *string) (string, error)
	DeleteRpmRepository(rpmRepositoryHref string) (string, error)

	// Rpm Repository Version
	GetRpmRepositoryVersion(href string) (*zest.RepositoryVersionResponse, error)
	DeleteRpmRepositoryVersion(href string) (string, error)

	// RpmPublication
	CreateRpmPublication(versionHref string) (*string, error)
	FindRpmPublicationByVersion(versionHref string) (*zest.RpmRpmPublicationResponse, error)

	// Distribution
	CreateRpmDistribution(publicationHref string, name string, basePath string) (*string, error)
	FindDistributionByPath(path string) (*zest.RpmRpmDistributionResponse, error)
	DeleteRpmDistribution(rpmDistributionHref string) (string, error)

	// Domains
	LookupOrCreateDomain(name string) (string, error)
	LookupDomain(name string) (string, error)
	UpdateDomainIfNeeded(name string) error

	// Status
	Status() (*zest.StatusResponse, error)
}
