package pulp_client

import (
	zest "github.com/content-services/zest/release/v3"
	"github.com/stretchr/testify/mock"
)

type MockPulpClient struct {
	mock.Mock
}

func (r *MockPulpClient) CreateRpmRepository(uuid string, url string, rpmRemotePulpRef *string) (zest.RpmRpmRepositoryResponse, error) {
	args := r.Called(uuid, url, *rpmRemotePulpRef)
	if v, ok := args.Get(0).(zest.RpmRpmRepositoryResponse); ok {
		return v, nil
	}
	return zest.RpmRpmRepositoryResponse{}, nil
}

func (r *MockPulpClient) GetRpmRepositoryByName(name string) (zest.RpmRpmRepositoryResponse, error) {
	args := r.Called(name)
	if v, ok := args.Get(0).(zest.RpmRpmRepositoryResponse); ok {
		return v, nil
	}
	return zest.RpmRpmRepositoryResponse{}, nil
}

func (r *MockPulpClient) GetRpmRepositoryByRemote(pulpHref string) (zest.RpmRpmRepositoryResponse, error) {
	args := r.Called(pulpHref)
	if v, ok := args.Get(0).(zest.RpmRpmRepositoryResponse); ok {
		return v, nil
	}
	return zest.RpmRpmRepositoryResponse{}, nil
}

func (r *MockPulpClient) SyncRpmRepository(rpmRpmRepositoryHref string, remoteHref *string) (string, error) {
	args := r.Called(rpmRpmRepositoryHref)
	if v, ok := args.Get(0).(string); ok {
		return v, nil
	}
	return "taskPulpHref", nil
}
