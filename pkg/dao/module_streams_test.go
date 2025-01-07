package dao

import (
	"context"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/seeds"
	"github.com/content-services/tang/pkg/tangy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type ModuleStreamSuite struct {
	*DaoSuite
	repoConfig  *models.RepositoryConfiguration
	repo        *models.Repository
	repoPrivate *models.Repository
}

func (s *ModuleStreamSuite) SetupTest() {
	s.DaoSuite.SetupTest()

	repo := repoPublicTest.DeepCopy()
	if err := s.tx.Create(repo).Error; err != nil {
		s.FailNow("Preparing Repository record: %w", err)
	}
	s.repo = repo

	repoPrivate := repoPrivateTest.DeepCopy()
	if err := s.tx.Create(repoPrivate).Error; err != nil {
		s.FailNow("Preparing private Repository record: %w", err)
	}
	s.repoPrivate = repoPrivate

	repoConfig := repoConfigTest1.DeepCopy()
	repoConfig.RepositoryUUID = repo.Base.UUID
	if err := s.tx.Create(repoConfig).Error; err != nil {
		s.FailNow("Preparing RepositoryConfiguration record: %w", err)
	}
	s.repoConfig = repoConfig
}

func TestModuleStreamSuite(t *testing.T) {
	m := DaoSuite{}
	r := ModuleStreamSuite{DaoSuite: &m}
	suite.Run(t, &r)
}

func (s *RpmSuite) TestSearchModulesForSnapshots() {
	orgId := seeds.RandomOrgId()
	mTangy, origTangy := mockTangy(s.T())
	defer func() { config.Tang = origTangy }()
	ctx := context.Background()

	hrefs := []string{"some_pulp_version_href"}
	stream1 := tangy.ModuleStreams{
		Name: "Foodidly",
		// add more
	}
	expected := []tangy.ModuleStreams{stream1}

	// Create a repo config, and snapshot, update its version_href to expected href
	_, err := seeds.SeedRepositoryConfigurations(s.tx, 1, seeds.SeedOptions{
		OrgID:     orgId,
		BatchSize: 0,
	})
	require.NoError(s.T(), err)
	repoConfig := models.RepositoryConfiguration{}
	res := s.tx.Where("org_id = ?", orgId).First(&repoConfig)
	require.NoError(s.T(), res.Error)
	snaps, err := seeds.SeedSnapshots(s.tx, repoConfig.UUID, 1)
	require.NoError(s.T(), err)
	res = s.tx.Model(&models.Snapshot{}).Where("repository_configuration_uuid = ?", repoConfig.UUID).Update("version_href", hrefs[0])
	require.NoError(s.T(), res.Error)
	// pulpHrefs, request.Search, *request.Limit)
	mTangy.On("RpmRepositoryVersionModuleStreamsList", ctx, hrefs, tangy.ModuleStreamListFilters{Search: "Foo", RpmNames: []string{}}, "").Return(expected, nil)
	//ctx context.Context, hrefs []string, rpmNames []string, search string, pageOpts PageOption
	dao := GetModuleStreamsDao(s.tx)

	resp, err := dao.SearchSnapshotModuleStreams(ctx, orgId, api.SearchSnapshotModuleStreamsRequest{
		UUIDs:    []string{snaps[0].UUID},
		RpmNames: []string(nil),
		Search:   "Foo",
	})

	require.NoError(s.T(), err)

	assert.Equal(s.T(),
		[]api.SearchModuleStreams{{ModuleName: expected[0].Name, Streams: []api.Stream{{Name: stream1.Name}}}},
		resp,
	)

	// ensure error returned for invalid snapshot uuid
	_, err = dao.SearchSnapshotModuleStreams(ctx, orgId, api.SearchSnapshotModuleStreamsRequest{
		UUIDs:  []string{"blerg!"},
		Search: "Foo",
	})

	assert.Error(s.T(), err)

	// ensure error returned for no uuids
	_, err = dao.SearchSnapshotModuleStreams(ctx, orgId, api.SearchSnapshotModuleStreamsRequest{
		UUIDs:    []string{},
		RpmNames: []string{},
		Search:   "Foo",
	})

	assert.Error(s.T(), err)
}
