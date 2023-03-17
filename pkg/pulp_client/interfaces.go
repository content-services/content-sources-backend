package pulp_client

import zest "github.com/content-services/zest/release/v3"

type PulpClient interface {
	//Remotes
	CreateRpmRemote(name string, url string) (*zest.RpmRpmRemoteResponse, error)
	UpdateRpmRemoteUrl(pulpHref string, url string) (string, error)
	GetRpmRemoteByName(name string) (zest.RpmRpmRemoteResponse, error)
	GetRpmRemoteList() ([]zest.RpmRpmRemoteResponse, error)
	DeleteRpmRemote(pulpHref string) (string, error)
	//Tasks
	GetTask(taskHref string) (zest.TaskResponse, error)
	//Rpm
	CreateRpmRepository(uuid string, url string, rpmRemotePulpRef *string) (zest.RpmRpmRepositoryResponse, error)
	GetRpmRepositoryByName(name string) (zest.RpmRpmRepositoryResponse, error)
	GetRpmRepositoryByRemote(pulpHref string) (zest.RpmRpmRepositoryResponse, error)
	SyncRpmRepository(rpmRpmRepositoryHref string, remoteHref *string) (string, error)
}
