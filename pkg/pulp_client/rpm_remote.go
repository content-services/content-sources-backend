package pulp_client

import zest "github.com/content-services/zest/release/v3"

// Creates a remote
func (r pulpDaoImpl) CreateRpmRemote(name string, url string) (*zest.RpmRpmRemoteResponse, error) {
	rpmRpmRemote := *zest.NewRpmRpmRemote(name, url)
	rpmRpmRemote.SetPolicy(zest.POLICY762ENUM_ON_DEMAND)
	remoteResp, _, err := r.client.RemotesRpmApi.RemotesRpmRpmCreate(r.ctx).
		RpmRpmRemote(rpmRpmRemote).Execute()

	if err != nil {
		return nil, err
	}

	return remoteResp, nil
}

// Starts an update task on an existing remote
func (r pulpDaoImpl) UpdateRpmRemoteUrl(pulpHref string, url string) (string, error) {
	patchRpmRemote := zest.PatchedrpmRpmRemote{}
	patchRpmRemote.SetUrl(url)
	updateResp, _, err := r.client.RemotesRpmApi.RemotesRpmRpmPartialUpdate(r.ctx, pulpHref).
		PatchedrpmRpmRemote(patchRpmRemote).Execute()

	if err != nil {
		return "", err
	}

	return updateResp.Task, nil
}

// Finds a remote by name, returning the associated RpmRpmRemoteResponse (containing the PulpHref)
func (r pulpDaoImpl) GetRpmRemoteByName(name string) (zest.RpmRpmRemoteResponse, error) {
	readResp, _, err := r.client.RemotesRpmApi.RemotesRpmRpmList(r.ctx).Name(name).Execute()

	if err != nil {
		return zest.RpmRpmRemoteResponse{}, err
	}

	results := readResp.GetResults()
	return results[0], nil
}

// Returns a list of RpmRpmRemotes
func (r pulpDaoImpl) GetRpmRemoteList() ([]zest.RpmRpmRemoteResponse, error) {
	readResp, _, err := r.client.RemotesRpmApi.RemotesRpmRpmList(r.ctx).Execute()

	if err != nil {
		return []zest.RpmRpmRemoteResponse{}, err
	}

	results := readResp.GetResults()
	return results, nil
}

// Starts a Delete task on an existing remote
func (r pulpDaoImpl) DeleteRpmRemote(pulpHref string) (string, error) {
	deleteResp, _, err := r.client.RemotesRpmApi.RemotesRpmRpmDelete(r.ctx, pulpHref).Execute()

	if err != nil {
		return "", err
	}

	return deleteResp.Task, nil
}
