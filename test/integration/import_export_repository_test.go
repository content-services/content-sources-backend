package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/handler"
	"github.com/content-services/content-sources-backend/pkg/middleware"
	test_handler "github.com/content-services/content-sources-backend/pkg/test/handler"
	"github.com/labstack/echo/v4"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type ImportExportRepoSuite struct {
	Suite
	ctx      context.Context
	server   *http.Server
	identity identity.XRHID
	cancel   context.CancelFunc
}

func (s *ImportExportRepoSuite) SetupTest() {
	s.Suite.SetupTest()
	s.ctx, s.cancel = context.WithCancel(context.Background())

	config.Get().Features.Snapshots.Enabled = true

	err := db.Connect()
	require.NoError(s.T(), err)

	router := echo.New()
	router.HTTPErrorHandler = config.CustomHTTPErrorHandler
	router.Use(middleware.WrapMiddlewareWithSkipper(identity.EnforceIdentity, middleware.SkipAuth))

	handler.RegisterRoutes(router)

	s.server = &http.Server{
		Addr:              "127.0.0.1:8100",
		Handler:           router,
		IdleTimeout:       1 * time.Minute,
		ReadTimeout:       5 * time.Second,
		ReadHeaderTimeout: 2 * time.Second,
		WriteTimeout:      10 * time.Second,
	}

	// force local storage for integration tests
	config.Get().Clients.Pulp.StorageType = "local"
}

func (s *ImportExportRepoSuite) TearDownTest() {
	s.cancel()
	err := s.server.Shutdown(context.Background())
	if err != nil {
		log.Error().Err(err).Msg("Could not shutdown server")
	}
	s.Suite.TearDownTest()
}

func (s *ImportExportRepoSuite) serveRepositoryRouter(req *http.Request) (int, []byte, error) {
	rr := httptest.NewRecorder()
	s.server.Handler.ServeHTTP(rr, req)

	response := rr.Result()
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	require.NoError(s.T(), err)

	return response.StatusCode, body, err
}

func TestImportExportRepoSuite(t *testing.T) {
	suite.Run(t, new(ImportExportRepoSuite))
}

func (s *ImportExportRepoSuite) TestExportAndImportToDifferentOrg() {
	t := s.T()

	// set first org id
	orgId1 := fmt.Sprintf("ExportAndImport-%v", rand.Int())
	s.identity = test_handler.MockIdentity
	s.identity.Identity.OrgID = orgId1
	s.identity.Identity.Internal.OrgID = orgId1

	// create repos
	repoUrls := []string{"https://fedorapeople.org/groups/katello/fakerepos/zoo/", "https://fedorapeople.org/groups/katello/fakerepos/zoo2/"}
	repoUuids := []string{}
	for i := 0; i < len(repoUrls); i++ {
		repo := s.createAndSyncRepository(s.identity.Identity.OrgID, repoUrls[i])
		repoUuids = append(repoUuids, repo.UUID)
	}

	// export repos from first org
	exportedRepos := s.bulkExportRepositories(repoUuids)
	assert.Equal(t, len(exportedRepos), len(repoUuids))

	// set second org id
	orgId2 := fmt.Sprintf("ExportAndImport-%v", rand.Int())
	s.identity.Identity.OrgID = orgId2
	s.identity.Identity.Internal.OrgID = orgId2

	// import repos into second org
	importedRepos := s.bulkImportRepositories(exportedRepos)
	assert.Equal(t, len(importedRepos), len(exportedRepos))

	for i := 0; i < len(importedRepos); i++ {
		assert.Equal(t, importedRepos[i].OrgID, orgId2)
	}
}

func (s *ImportExportRepoSuite) bulkExportRepositories(repoUuids []string) []api.RepositoryExportResponse {
	t := s.T()
	request := api.RepositoryExportRequest{
		RepositoryUuids: repoUuids,
	}

	body, err := json.Marshal(request)
	require.NoError(t, err)
	path := api.FullRootPath() + "/repositories/bulk_export/"
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Header.Set(api.IdentityHeader, test_handler.EncodedCustomIdentity(t, s.identity))
	req.Header.Set("Content-Type", "application/json")
	code, body, err := s.serveRepositoryRouter(req)
	require.NoError(t, err, "failure exporting repos")
	assert.Equal(t, http.StatusOK, code, string(body))
	repoResp := []api.RepositoryExportResponse{}
	err = json.Unmarshal(body, &repoResp)
	assert.Nil(t, err)

	return repoResp
}

func (s *ImportExportRepoSuite) bulkImportRepositories(repos []api.RepositoryExportResponse) []api.RepositoryImportResponse {
	t := s.T()
	amountToImport := len(repos)

	requests := make([]api.RepositoryRequest, amountToImport)
	for i := 0; i < amountToImport; i++ {
		requests[i] = api.RepositoryRequest{
			Name:                 &repos[i].Name,
			URL:                  &repos[i].URL,
			DistributionVersions: &repos[i].DistributionVersions,
			DistributionArch:     &repos[i].DistributionArch,
			GpgKey:               &repos[i].GpgKey,
			MetadataVerification: &repos[i].MetadataVerification,
			ModuleHotfixes:       &repos[i].ModuleHotfixes,
			Snapshot:             &repos[i].Snapshot,
		}
	}

	body, err := json.Marshal(requests)
	require.NoError(t, err)
	path := api.FullRootPath() + "/repositories/bulk_import/"
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Header.Set(api.IdentityHeader, test_handler.EncodedCustomIdentity(t, s.identity))
	req.Header.Set("Content-Type", "application/json")
	code, body, err := s.serveRepositoryRouter(req)
	require.NoError(t, err, "failure importing repos")
	assert.Equal(t, http.StatusCreated, code, string(body))
	repoResp := []api.RepositoryImportResponse{}
	err = json.Unmarshal(body, &repoResp)
	assert.Nil(t, err)

	return repoResp
}
