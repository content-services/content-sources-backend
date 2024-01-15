package pulp_client

import (
	"fmt"
	"io"

	zest "github.com/content-services/zest/release/v2023"
	"github.com/rs/zerolog/log"
)

// Creates a repository, rpmRemotePulpRef is optional
func (r *pulpDaoImpl) CreateRpmRepository(uuid string, rpmRemotePulpRef *string) (*zest.RpmRpmRepositoryResponse, error) {
	rpmRpmRepository := *zest.NewRpmRpmRepository(uuid)
	if rpmRemotePulpRef != nil {
		rpmRpmRepository.SetRemote(*rpmRemotePulpRef)
	}
	resp, httpResp, err := r.client.RepositoriesRpmAPI.RepositoriesRpmRpmCreate(r.ctx, r.domainName).
		RpmRpmRepository(rpmRpmRepository).Execute()

	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()

	return resp, nil
}

// Finds a repository given a name
func (r *pulpDaoImpl) GetRpmRepositoryByName(name string) (*zest.RpmRpmRepositoryResponse, error) {
	resp, httpResp, err := r.client.RepositoriesRpmAPI.RepositoriesRpmRpmList(r.ctx, r.domainName).Name(name).Execute()

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
	resp, httpResp, err := r.client.RepositoriesRpmAPI.RepositoriesRpmRpmList(r.ctx, r.domainName).Remote(pulpHref).Execute()

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
	resp, httpResp, err := r.client.RepositoriesRpmAPI.RepositoriesRpmRpmSync(r.ctx, rpmRpmRepositoryHref).
		RpmRepositorySyncURL(rpmRepositoryHref).Execute()
	defer httpResp.Body.Close()

	if err != nil {
		if httpResp != nil {
			body, readErr := io.ReadAll(httpResp.Body)
			if readErr == nil {
				return "", fmt.Errorf("error starting sync %w: %v", err, string(body[:]))
			} else {
				log.Logger.Error().Err(readErr).Msg("could not read http body")
			}
			return "", fmt.Errorf("error starting sync %w: %v", err, string(body[:]))
		} else {
			return "", err
		}
	}

	return resp.Task, nil
}

// DeleteRpmRepository starts task to delete an rpm repository and returns the delete task href
func (r *pulpDaoImpl) DeleteRpmRepository(rpmRepositoryHref string) (string, error) {
	resp, httpResp, err := r.client.RepositoriesRpmAPI.RepositoriesRpmRpmDelete(r.ctx, rpmRepositoryHref).Execute()
	if err != nil {
		return "", err
	}
	defer httpResp.Body.Close()
	return resp.Task, nil
}
