package external_repos

import (
	"context"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	zest "github.com/content-services/zest/release/v2026"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func withLightwellIgnoreRepos(t *testing.T, names []string) {
	t.Helper()
	orig := config.Get().Options.LightwellIgnoreRepos
	config.Get().Options.LightwellIgnoreRepos = names
	t.Cleanup(func() { config.Get().Options.LightwellIgnoreRepos = orig })
}

func TestImportEntriesSkipsIgnoredRepos(t *testing.T) {
	withLightwellIgnoreRepos(t, []string{"lightwell/npm/validated", "lightwell/npm/remediated"})

	daoReg := dao.GetMockDaoRegistry(t)
	pulp := pulp_client.NewMockPulpClient(t)

	publishedURL := "https://pulp.example.com/python/validated"
	pulp.On("FindGenericDistributionByBasePath", mock.Anything, "python/validated").
		Return(&zest.DistributionResponse{BaseUrl: &publishedURL}, nil)
	daoReg.RepositoryConfig.On(
		"InternalOnly_RefreshLightwellRepo",
		mock.Anything,
		config.LightwellOrg,
		"lightwell/python/validated",
		"validated",
		"python",
		publishedURL,
		"python/validated",
		"lightwell-network",
	).Return(&api.RepositoryResponse{}, nil)

	importer := NewLightwellRepoImporter(daoReg.ToDaoRegistry(), pulp, nil)
	err := importer.importEntries(context.Background(), []LightwellAllowlistEntry{
		{Name: "lightwell/npm/validated", Type: "npm", BasePath: "npm/validated", FeatureName: "lightwell-network"},
		{Name: "lightwell/python/validated", Type: "python", BasePath: "python/validated", FeatureName: "lightwell-network"},
		{Name: "lightwell/npm/remediated", Type: "npm", BasePath: "npm/remediated", FeatureName: "lightwell-network"},
	}, pulp, config.LightwellOrg)
	require.NoError(t, err)

	pulp.AssertNotCalled(t, "FindGenericDistributionByBasePath", mock.Anything, "npm/validated")
	pulp.AssertNotCalled(t, "FindGenericDistributionByBasePath", mock.Anything, "npm/remediated")
	daoReg.RepositoryConfig.AssertNotCalled(t, "InternalOnly_RefreshLightwellRepo",
		mock.Anything, mock.Anything, "lightwell/npm/validated", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	daoReg.RepositoryConfig.AssertNotCalled(t, "InternalOnly_RefreshLightwellRepo",
		mock.Anything, mock.Anything, "lightwell/npm/remediated", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestImportEntriesMissingDistributionURL(t *testing.T) {
	withLightwellIgnoreRepos(t, nil)

	daoReg := dao.GetMockDaoRegistry(t)
	pulp := pulp_client.NewMockPulpClient(t)
	pulp.On("FindGenericDistributionByBasePath", mock.Anything, "npm/validated").
		Return((*zest.DistributionResponse)(nil), nil)

	importer := NewLightwellRepoImporter(daoReg.ToDaoRegistry(), pulp, nil)
	err := importer.importEntries(context.Background(), []LightwellAllowlistEntry{
		{Name: "lightwell/npm/validated", Type: "npm", BasePath: "npm/validated", FeatureName: "lightwell-network"},
	}, pulp, config.LightwellOrg)
	require.EqualError(t, err, `lightwell repo "lightwell/npm/validated" has no published distribution URL`)
}

func TestIgnoredLightwellRepoExactName(t *testing.T) {
	withLightwellIgnoreRepos(t, []string{"lightwell/npm/validated"})

	require.True(t, IgnoredLightwellRepo("lightwell/npm/validated"))
	require.False(t, IgnoredLightwellRepo("lightwell/npm/validated-demo"))
	require.False(t, IgnoredLightwellRepo("lightwell/npm/remediated"))
}
