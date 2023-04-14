package integration

import (
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/tasks"
	uuid2 "github.com/google/uuid"
	"github.com/openlyinc/pointy"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type SnapshotSuite struct {
	Suite
	dao *dao.DaoRegistry
}

func TestSnapshotSuite(t *testing.T) {
	suite.Run(t, new(SnapshotSuite))
}

func (s *SnapshotSuite) TestSnapshot() {
	log.Logger.Error().Msgf("USERNAME: %v", config.Get().Clients.Pulp.Username)
	log.Logger.Error().Msgf("Password: %v", config.Get().Clients.Pulp.Password)
	s.dao = dao.GetDaoRegistry(s.db)
	uuid := uuid2.NewString()
	accountId := uuid2.NewString()

	repo, err := s.dao.RepositoryConfig.Create(api.RepositoryRequest{
		Name:      pointy.String(uuid),
		URL:       pointy.String("https://fixtures.pulpproject.org/rpm-unsigned/"),
		AccountID: pointy.String(accountId),
		OrgID:     pointy.String(accountId),
	})

	assert.NoError(s.T(), err)

	err = tasks.SnapshotRepository(tasks.SnapshotOptions{
		OrgId:          accountId,
		RepoConfigUuid: repo.UUID,
		DaoRegistry:    s.dao,
		PulpClient:     pulp_client.GetPulpClient(),
	})
	assert.NoError(s.T(), err)

	snaps, err := s.dao.Snapshot.List(repo)
	assert.NoError(s.T(), err)
	assert.NotEmpty(s.T(), snaps)
	time.Sleep(5 * time.Second)

	distPath := fmt.Sprintf("%s/pulp/content/%s/repodata/repomd.xml",
		config.Get().Clients.Pulp.Server,
		snaps[0].DistributionPath)

	resp, err := http.Get(distPath)
	assert.NoError(s.T(), err)
	defer resp.Body.Close()
	assert.Equal(s.T(), resp.StatusCode, 200)
	body, err := io.ReadAll(resp.Body)
	assert.NoError(s.T(), err)
	assert.NotEmpty(s.T(), body)
}
