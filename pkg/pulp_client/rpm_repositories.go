package pulp_client

import zest "github.com/content-services/zest/release/v3"

// Creates a repository, rpmRemotePulpRef is optional
func (r *pulpDaoImpl) CreateRpmRepository(uuid string, rpmRemotePulpRef *string) (*zest.RpmRpmRepositoryResponse, error) {
	rpmRpmRepository := *zest.NewRpmRpmRepository(uuid)
	if rpmRemotePulpRef != nil {
		rpmRpmRepository.SetRemote(*rpmRemotePulpRef)
	}
	resp, httpResp, err := r.client.RepositoriesRpmApi.RepositoriesRpmRpmCreate(r.ctx).
		RpmRpmRepository(rpmRpmRepository).Execute()

	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()

	return resp, nil
}

// Finds a repository given a name
func (r *pulpDaoImpl) GetRpmRepositoryByName(name string) (*zest.RpmRpmRepositoryResponse, error) {
	resp, httpResp, err := r.client.RepositoriesRpmApi.RepositoriesRpmRpmList(r.ctx).Name(name).Execute()

	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()

	results := resp.GetResults()
	if len(results) > 0 {
		return &results[0], nil
	} else {
		return nil, nil
	}
}

// Finds a repository given a remoteHref
func (r *pulpDaoImpl) GetRpmRepositoryByRemote(pulpHref string) (*zest.RpmRpmRepositoryResponse, error) {
	resp, httpResp, err := r.client.RepositoriesRpmApi.RepositoriesRpmRpmList(r.ctx).Remote(pulpHref).Execute()

	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()

	results := resp.GetResults()
	if len(results) > 0 {
		return &results[0], nil
	} else {
		return nil, nil
	}
}

// Starts a sync task, returns a taskHref, remoteHref is optional.
func (r *pulpDaoImpl) SyncRpmRepository(rpmRpmRepositoryHref string, remoteHref *string) (string, error) {
	rpmRepositoryHref := *zest.NewRpmRepositorySyncURL()
	if remoteHref != nil {
		rpmRepositoryHref.SetRemote(*remoteHref)
	}
	rpmRepositoryHref.SetSyncPolicy(*zest.SYNCPOLICYENUM_MIRROR_CONTENT_ONLY.Ptr())
	resp, httpResp, err := r.client.RepositoriesRpmApi.RepositoriesRpmRpmSync(r.ctx, rpmRpmRepositoryHref).
		RpmRepositorySyncURL(rpmRepositoryHref).Execute()
	defer httpResp.Body.Close()

	if err != nil {
		return "", err
	}

	return resp.Task, nil
}
