package pulp_client

import zest "github.com/content-services/zest/release/v3"

func (r *MockPulpClient) CreateRpmRemote(name string, url string) (*zest.RpmRpmRemoteResponse, error) {
	args := r.Called(name, url)

	if v, ok := args.Get(0).(*zest.RpmRpmRemoteResponse); ok {
		return v, nil
	}
	response := zest.RpmRpmRemoteResponse{Name: name, Url: url}
	response.SetPulpHref("remotePulpHref")
	return &response, args.Error(1)
}

func (r *MockPulpClient) UpdateRpmRemoteUrl(pulpHref string, url string) (string, error) {
	args := r.Called(pulpHref, url)
	if v, ok := args.Get(0).(string); ok {
		return v, nil
	}
	return "", nil
}

func (r *MockPulpClient) GetRpmRemoteByName(name string) (zest.RpmRpmRemoteResponse, error) {
	args := r.Called(name)
	if v, ok := args.Get(0).(zest.RpmRpmRemoteResponse); ok {
		return v, nil
	}
	return zest.RpmRpmRemoteResponse{}, nil
}

func (r *MockPulpClient) GetRpmRemoteList() ([]zest.RpmRpmRemoteResponse, error) {
	args := r.Called()
	if v, ok := args.Get(0).([]zest.RpmRpmRemoteResponse); ok {
		return v, nil
	}
	return []zest.RpmRpmRemoteResponse{}, nil
}

func (r *MockPulpClient) DeleteRpmRemote(pulpHref string) (string, error) {
	args := r.Called(pulpHref)
	if v, ok := args.Get(0).(string); ok {
		return v, nil
	}
	return "", nil
}
