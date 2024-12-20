package dao

import (
	"context"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/seeds"
	"github.com/content-services/tang/pkg/tangy"
	"github.com/content-services/yummy/pkg/yum"
	"github.com/labstack/gommon/random"
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
	// ctx context.Context, hrefs []string, rpmNames []string, search string, pageOpts PageOption
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

func testYumModuleMD() yum.ModuleMD {
	return yum.ModuleMD{
		Document: "",
		Version:  0,
		Data: yum.Stream{
			Name:        "myModule",
			Stream:      "myStream",
			Version:     "Version",
			Context:     "lksdfoisdjf",
			Arch:        "x86_64",
			Summary:     "something short",
			Description: "something long",
			Profiles:    map[string]yum.RpmProfiles{"common": {Rpms: []string{"foo"}}},
			Artifacts: yum.Artifacts{Rpms: []string{"ruby-0:2.5.5-106.module+el8.3.0+7153+c6f6daa5.i686",
				"ruby-irb-0:2.5.5-106.module+el8.3.0+7153+c6f6daa5.noarch"}},
		},
	}
}

func (s *ModuleStreamSuite) TestSearchRepositoryModuleStreams() {
	t := s.Suite.T()
	tx := s.tx
	var err error
	dao := GetModuleStreamsDao(tx)

	alpha1 := genModule("alpha", "1.0", "123")
	alpha2 := genModule("alpha", "1.0", "124")
	alphaNew := genModule("alpha", "1.1", "126")
	unrel := genModule("unrelated", "1.1", "123")
	beta1 := genModule("beta", "1.1", "123")

	err = tx.Create([]*models.ModuleStream{&alpha1, &alpha2, &beta1, &alphaNew, &unrel}).Error
	require.NoError(t, err)
	err = tx.Create([]models.RepositoryModuleStream{
		{RepositoryUUID: s.repo.UUID, ModuleStreamUUID: alpha1.UUID},
		{RepositoryUUID: s.repo.UUID, ModuleStreamUUID: alpha2.UUID},
		{RepositoryUUID: s.repo.UUID, ModuleStreamUUID: alphaNew.UUID},
		{RepositoryUUID: s.repo.UUID, ModuleStreamUUID: beta1.UUID},
	}).Error
	require.NoError(t, err)

	resp, err := dao.SearchRepositoryModuleStreams(context.Background(), s.repoConfig.OrgID, api.SearchModuleStreamsRequest{
		UUIDs: []string{s.repoConfig.UUID},
	})
	assert.NoError(t, err)

	// 2 modules in total, 3rd isn't in the repo
	assert.Equal(t, 2, len(resp))

	// alpha module has 2 streams
	assert.Equal(t, alpha2.Name, resp[0].ModuleName)
	assert.Equal(t, 2, len(resp[0].Streams))
	assert.Equal(t, alpha2.Version, resp[0].Streams[0].Version)
	assert.Equal(t, alphaNew.Version, resp[0].Streams[1].Version)

	// only 1 stream for beta module
	assert.Equal(t, 1, len(resp[1].Streams))
	assert.Equal(t, beta1.Name, resp[1].ModuleName)
	assert.Equal(t, beta1.Version, resp[1].Streams[0].Version)

	// reverse order
	resp, err = dao.SearchRepositoryModuleStreams(context.Background(), s.repoConfig.OrgID, api.SearchModuleStreamsRequest{
		UUIDs:  []string{s.repoConfig.UUID},
		SortBy: "name:desc",
	})
	assert.NoError(t, err)
	assert.Equal(t, beta1.Name, resp[0].ModuleName) // beta comes first
	assert.Equal(t, alpha2.Name, resp[1].ModuleName)

	// URL
	resp, err = dao.SearchRepositoryModuleStreams(context.Background(), s.repoConfig.OrgID, api.SearchModuleStreamsRequest{
		URLs: []string{s.repo.URL},
	})
	assert.NoError(t, err)
	assert.Equal(t, 2, len(resp))

	// test rpm name search
	resp, err = dao.SearchRepositoryModuleStreams(context.Background(), s.repoConfig.OrgID, api.SearchModuleStreamsRequest{
		UUIDs:    []string{s.repoConfig.UUID},
		RpmNames: []string{alpha1.PackageNames[0], alpha2.PackageNames[0]},
	})
	assert.NoError(t, err)
	assert.Equal(t, 1, len(resp))
	assert.Equal(t, alpha2.Name, resp[0].ModuleName)
}

func genModule(name string, stream string, version string) models.ModuleStream {
	return models.ModuleStream{
		Name:         name,
		Stream:       stream,
		Version:      version,
		Context:      "context " + random.String(5),
		Arch:         "x86_64",
		Summary:      "summary:" + random.String(5),
		Description:  "desc:" + random.String(5),
		PackageNames: []string{random.String(5), random.String(5)},
		HashValue:    random.String(10),
	}
}

func (s *ModuleStreamSuite) TestInsertForRepository() {
	t := s.Suite.T()
	tx := s.tx

	mods := []yum.ModuleMD{testYumModuleMD()}

	dao := GetModuleStreamsDao(tx)
	cnt, err := dao.InsertForRepository(context.Background(), s.repo.UUID, mods)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), cnt)

	created := models.ModuleStream{}
	res := tx.Where("context = ?", mods[0].Data.Context).Find(&created)
	assert.NoError(t, res.Error)
	assert.NotEmpty(t, created.UUID)
	assert.Equal(t, created.PackageNames[0], "ruby")
	assert.Equal(t, created.PackageNames[1], "ruby-irb")

	pkgs, ok := created.Profiles["common"]
	assert.True(t, ok)
	assert.Len(t, pkgs, 1)
	assert.Equal(t, created.PackageNames[0], "ruby")

	assert.Len(t, created.Packages, 2)

	// re-run and expect only 1
	cnt, err = dao.InsertForRepository(context.Background(), s.repo.UUID, mods)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), cnt)
	var count int64
	res = tx.Model(&models.ModuleStream{}).Where("context = ?", mods[0].Data.Context).Count(&count)
	assert.NoError(t, res.Error)
	assert.Equal(t, int64(1), count)
}

func (s *ModuleStreamSuite) TestOrphanCleanup() {
	mod1 := models.ModuleStream{
		Name:         "mod1",
		Stream:       "mod1",
		Version:      "123",
		Context:      "mod1",
		Arch:         "mod1",
		Summary:      "mod1",
		Description:  "mod1",
		PackageNames: []string{"foo1"},
		HashValue:    random.String(10),
		Repositories: nil,
	}
	mod2 := models.ModuleStream{
		Name:         "mod2",
		Stream:       "mod2",
		Version:      "123",
		Context:      "mod2",
		Arch:         "mod2",
		Summary:      "mod2",
		Description:  "mod12",
		PackageNames: []string{"foo2"},
		HashValue:    random.String(10),
		Repositories: nil,
	}

	require.NoError(s.T(), s.tx.Create(&mod1).Error)
	require.NoError(s.T(), s.tx.Create(&mod2).Error)

	repos, err := seeds.SeedRepositoryConfigurations(s.tx, 1, seeds.SeedOptions{})
	require.NoError(s.T(), err)
	repo := repos[0]

	err = s.tx.Create(&models.RepositoryModuleStream{
		RepositoryUUID:   repo.RepositoryUUID,
		ModuleStreamUUID: mod1.UUID,
	}).Error
	require.NoError(s.T(), err)

	dao := GetModuleStreamsDao(s.tx)
	err = dao.OrphanCleanup(context.Background())
	require.NoError(s.T(), err)

	// verify mod1 exists and mod2 doesn't
	mods := []models.ModuleStream{}
	err = s.tx.Where("uuid in (?)", []string{mod1.UUID, mod2.UUID}).Find(&mods).Error
	require.NoError(s.T(), err)

	assert.Len(s.T(), mods, 1)
	assert.Equal(s.T(), mod1.UUID, mods[0].UUID)
}
