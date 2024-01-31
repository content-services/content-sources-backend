package pulp_client

import (
	"github.com/content-services/content-sources-backend/pkg/config"
	zest "github.com/content-services/zest/release/v2024"
	"github.com/rs/zerolog/log"
)

const DownloadPolicyOnDemand = "on_demand"
const DownloadPolicyImmediate = "immediate"

// Creates a remote
func (r *pulpDaoImpl) CreateRpmRemote(name string, url string, clientCert *string, clientKey *string, caCert *string) (*zest.RpmRpmRemoteResponse, error) {
	rpmRpmRemote := *zest.NewRpmRpmRemote(name, url)
	if clientCert != nil {
		rpmRpmRemote.SetClientCert(*clientCert)
	}
	if clientKey != nil {
		rpmRpmRemote.SetClientKey(*clientKey)
	}
	if caCert != nil {
		rpmRpmRemote.SetCaCert(*caCert)
	}

	policy := config.Get().Clients.Pulp.DownloadPolicy
	if policy == DownloadPolicyOnDemand {
		rpmRpmRemote.SetPolicy(zest.POLICY762ENUM_ON_DEMAND)
	} else if policy == DownloadPolicyImmediate {
		rpmRpmRemote.SetPolicy(zest.POLICY762ENUM_IMMEDIATE)
	} else {
		log.Logger.Error().Msgf("Unknown download policy %v, defaulting to Immediate", policy)
		rpmRpmRemote.SetPolicy(zest.POLICY762ENUM_IMMEDIATE)
	}

	remoteResp, httpResp, err := r.client.RemotesRpmAPI.RemotesRpmRpmCreate(r.ctx, r.domainName).
		RpmRpmRemote(rpmRpmRemote).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return nil, errorWithResponseBody("error creating rpm remote", httpResp, err)
	}

	return remoteResp, nil
}

// Starts an update task on an existing remote
func (r *pulpDaoImpl) UpdateRpmRemote(pulpHref string, url string, clientCert *string, clientKey *string, caCert *string) (string, error) {
	patchRpmRemote := zest.PatchedrpmRpmRemote{}
	if clientCert != nil {
		patchRpmRemote.SetClientCert(*clientCert)
	}
	if clientKey != nil {
		patchRpmRemote.SetClientKey(*clientKey)
	}
	if caCert != nil {
		patchRpmRemote.SetCaCert(*caCert)
	}

	patchRpmRemote.SetUrl(url)
	updateResp, httpResp, err := r.client.RemotesRpmAPI.RemotesRpmRpmPartialUpdate(r.ctx, pulpHref).
		PatchedrpmRpmRemote(patchRpmRemote).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return "", errorWithResponseBody("error in rpm remote partial update", httpResp, err)
	}
	defer httpResp.Body.Close()

	return updateResp.Task, nil
}

// Finds a remote by name, returning the associated RpmRpmRemoteResponse (containing the PulpHref)
func (r *pulpDaoImpl) GetRpmRemoteByName(name string) (*zest.RpmRpmRemoteResponse, error) {
	readResp, httpResp, err := r.client.RemotesRpmAPI.RemotesRpmRpmList(r.ctx, r.domainName).Name(name).Execute()
	if httpResp != nil {
		httpResp.Body.Close()
	}
	if err != nil {
		return nil, errorWithResponseBody("error listing rpm remotes", httpResp, err)
	}
	defer httpResp.Body.Close()

	results := readResp.GetResults()
	if len(results) > 0 {
		return &results[0], nil
	} else {
		return nil, nil
	}
}

// Returns a list of RpmRpmRemotes
func (r *pulpDaoImpl) GetRpmRemoteList() ([]zest.RpmRpmRemoteResponse, error) {
	readResp, httpResp, err := r.client.RemotesRpmAPI.RemotesRpmRpmList(r.ctx, r.domainName).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return []zest.RpmRpmRemoteResponse{}, errorWithResponseBody("listing rpm remotes", httpResp, err)
	}

	results := readResp.GetResults()
	return results, nil
}

// Starts a Delete task on an existing remote
func (r *pulpDaoImpl) DeleteRpmRemote(pulpHref string) (string, error) {
	deleteResp, httpResp, err := r.client.RemotesRpmAPI.RemotesRpmRpmDelete(r.ctx, pulpHref).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return "", errorWithResponseBody("error deleting rpm remote", httpResp, err)
	}
	defer httpResp.Body.Close()

	return deleteResp.Task, nil
}
