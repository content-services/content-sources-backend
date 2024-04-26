package pulp_client

import (
	"context"

	zest "github.com/content-services/zest/release/v2024"
)

// Creates a repository, rpmRemotePulpRef is optional
func (r *pulpDaoImpl) CreateRpmRepository(ctx context.Context, uuid string, rpmRemotePulpRef *string) (*zest.RpmRpmRepositoryResponse, error) {
	ctx, client := getZestClient(ctx)
	rpmRpmRepository := *zest.NewRpmRpmRepository(uuid)
	if rpmRemotePulpRef != nil {
		rpmRpmRepository.SetRemote(*rpmRemotePulpRef)
	}
	resp, httpResp, err := client.RepositoriesRpmAPI.RepositoriesRpmRpmCreate(ctx, r.domainName).
		RpmRpmRepository(rpmRpmRepository).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return nil, errorWithResponseBody("error creating rpm repository", httpResp, err)
	}

	return resp, nil
}

// Finds a repository given a name
func (r *pulpDaoImpl) GetRpmRepositoryByName(ctx context.Context, name string) (*zest.RpmRpmRepositoryResponse, error) {
	ctx, client := getZestClient(ctx)
	resp, httpResp, err := client.RepositoriesRpmAPI.RepositoriesRpmRpmList(ctx, r.domainName).Name(name).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return nil, errorWithResponseBody("error listing rpm repositories", httpResp, err)
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
func (r *pulpDaoImpl) GetRpmRepositoryByRemote(ctx context.Context, pulpHref string) (*zest.RpmRpmRepositoryResponse, error) {
	ctx, client := getZestClient(ctx)
	resp, httpResp, err := client.RepositoriesRpmAPI.RepositoriesRpmRpmList(ctx, r.domainName).Remote(pulpHref).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return nil, errorWithResponseBody("error listing rpm repositories", httpResp, err)
	}

	results := resp.GetResults()
	if len(results) > 0 {
		return &results[0], nil
	} else {
		return nil, nil
	}
}

// Starts a sync task, returns a taskHref, remoteHref is optional.
func (r *pulpDaoImpl) SyncRpmRepository(ctx context.Context, rpmRpmRepositoryHref string, remoteHref *string) (string, error) {
	ctx, client := getZestClient(ctx)
	rpmRepositoryHref := *zest.NewRpmRepositorySyncURL()
	if remoteHref != nil {
		rpmRepositoryHref.SetRemote(*remoteHref)
	}
	rpmRepositoryHref.SetSyncPolicy(*zest.SYNCPOLICYENUM_MIRROR_CONTENT_ONLY.Ptr())
	resp, httpResp, err := client.RepositoriesRpmAPI.RepositoriesRpmRpmSync(ctx, rpmRpmRepositoryHref).
		RpmRepositorySyncURL(rpmRepositoryHref).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return "", errorWithResponseBody("error syncing rpm repository", httpResp, err)
	}

	return resp.Task, nil
}

// DeleteRpmRepository starts task to delete an rpm repository and returns the delete task href
func (r *pulpDaoImpl) DeleteRpmRepository(ctx context.Context, rpmRepositoryHref string) (string, error) {
	ctx, client := getZestClient(ctx)
	resp, httpResp, err := client.RepositoriesRpmAPI.RepositoriesRpmRpmDelete(ctx, rpmRepositoryHref).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return "", errorWithResponseBody("error deleting rpm repository", httpResp, err)
	}
	return resp.Task, nil
}
