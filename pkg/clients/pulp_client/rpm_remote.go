package pulp_client

import (
	"context"

	"github.com/content-services/content-sources-backend/pkg/config"
	zest "github.com/content-services/zest/release/v2025"
	"github.com/rs/zerolog/log"
)

const DownloadPolicyOnDemand = "on_demand"
const DownloadPolicyImmediate = "immediate"

// Creates a remote
func (r *pulpDaoImpl) CreateRpmRemote(ctx context.Context, name string, url string, clientCert *string, clientKey *string, caCert *string) (*zest.RpmRpmRemoteResponse, error) {
	ctx, client, err := getZestClient(ctx)
	if err != nil {
		return nil, err
	}

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
	switch policy {
	case DownloadPolicyOnDemand:
		rpmRpmRemote.SetPolicy(zest.POLICY692ENUM_ON_DEMAND)
	case DownloadPolicyImmediate:
		rpmRpmRemote.SetPolicy(zest.POLICY692ENUM_IMMEDIATE)
	default:
		log.Logger.Error().Msgf("Unknown download policy %v, defaulting to Immediate", policy)
		rpmRpmRemote.SetPolicy(zest.POLICY692ENUM_IMMEDIATE)
	}

	remoteResp, httpResp, err := client.RemotesRpmAPI.RemotesRpmRpmCreate(ctx, r.domainName).
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
func (r *pulpDaoImpl) UpdateRpmRemote(ctx context.Context, pulpHref string, url string, clientCert *string, clientKey *string, caCert *string) (string, error) {
	ctx, client, err := getZestClient(ctx)
	if err != nil {
		return "", err
	}

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

	// Execute returns as the first parameter either no taskHref and resp.StatusCode 200 or taskHref with resp.StatusCode 202
	updateResp, httpResp, err := client.RemotesRpmAPI.RemotesRpmRpmPartialUpdate(ctx, pulpHref).
		PatchedrpmRpmRemote(patchRpmRemote).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}

	// errorMsg is temporary workaround (zest throws error since it expects a pulpTaskHref to be always returned)
	// until zest gets updated upon pulp update
	var errorMsg string
	if err != nil {
		errorMsg = err.Error()
	}
	if err != nil && errorMsg != "no value given for required property task" {
		return "", errorWithResponseBody("error in rpm remote partial update", httpResp, err)
	}
	if httpResp != nil && httpResp.StatusCode == 200 {
		return "", nil
	}
	return updateResp.Task, nil
}

// Finds a remote by name, returning the associated RpmRpmRemoteResponse (containing the PulpHref)
func (r *pulpDaoImpl) GetRpmRemoteByName(ctx context.Context, name string) (*zest.RpmRpmRemoteResponse, error) {
	ctx, client, err := getZestClient(ctx)
	if err != nil {
		return nil, err
	}
	readResp, httpResp, err := client.RemotesRpmAPI.RemotesRpmRpmList(ctx, r.domainName).Name(name).Execute()
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
func (r *pulpDaoImpl) GetRpmRemoteList(ctx context.Context) ([]zest.RpmRpmRemoteResponse, error) {
	ctx, client, err := getZestClient(ctx)
	if err != nil {
		return []zest.RpmRpmRemoteResponse{}, err
	}
	readResp, httpResp, err := client.RemotesRpmAPI.RemotesRpmRpmList(ctx, r.domainName).Execute()
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
func (r *pulpDaoImpl) DeleteRpmRemote(ctx context.Context, pulpHref string) (string, error) {
	ctx, client, err := getZestClient(ctx)
	if err != nil {
		return "", err
	}

	deleteResp, httpResp, err := client.RemotesRpmAPI.RemotesRpmRpmDelete(ctx, pulpHref).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return "", errorWithResponseBody("error deleting rpm remote", httpResp, err)
	}

	return deleteResp.Task, nil
}
