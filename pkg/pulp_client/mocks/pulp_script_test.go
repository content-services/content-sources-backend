package pulp_client

import (
	"testing"

	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/external_repos"
	zest "github.com/content-services/zest/release/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestCreateRpmRemote(t *testing.T) {
	mockPulpClient := &MockPulpClient{}

	repoUUID := uuid.NewString()
	url := "http://someRandom.url/thing"
	expectedRemoteResponse := zest.RpmRpmRemoteResponse{}
	expectedRemoteResponse.SetPulpHref("remotePulpHref")
	expectedRepositoryResponse := zest.RpmRpmRepositoryResponse{}
	expectedRepositoryResponse.SetPulpHref("rpmPulpHref")
	mockPulpClient.On("CreateRpmRemote", repoUUID, url).Return(expectedRemoteResponse, nil)
	mockPulpClient.On("CreateRpmRepository", repoUUID, url, "remotePulpHref").Return(expectedRepositoryResponse, nil)
	mockPulpClient.On("SyncRpmRepository", "rpmPulpHref").Return("taskPulpHref", nil)
	mockPulpClient.On("GetTask", "taskPulpHref").Return("completed", nil)

	repository := &dao.Repository{UUID: repoUUID, URL: url}
	err := external_repos.PulpCreate(repository, mockPulpClient)
	assert.NoError(t, err)
}
