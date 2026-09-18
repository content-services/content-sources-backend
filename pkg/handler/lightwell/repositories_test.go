package lightwell

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/handler"
	test_handler "github.com/content-services/content-sources-backend/pkg/test/handler"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type LightwellReposSuite struct {
	LightwellSuite
}

func TestLightwellReposSuite(t *testing.T) {
	suite.Run(t, new(LightwellReposSuite))
}

func (s *LightwellReposSuite) TestRepositoriesRoute() {
	t := s.T()

	path := fmt.Sprintf("%s/repositories?limit=%d", LightwellAPIPath, 10)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := s.serveRouter(req)
	assert.Nil(t, err)
	// Route should be registered - we don't care about the exact response code
	assert.NotEqual(t, http.StatusNotFound, code, "Route should be registered")
}


func createRepoCollection(size, limit, offset int) api.RepositoryCollectionResponse {
	repos := make([]api.RepositoryResponse, size)
	for i := 0; i < size; i++ {
		repo := api.RepositoryResponse{
			UUID:         fmt.Sprintf("%d", i),
			Name:         fmt.Sprintf("repo_%d", i),
			URL:          fmt.Sprintf("http://repo-%d.com", i),
			AccountID:    test_handler.MockAccountNumber,
			OrgID:        test_handler.MockOrgId,
			Status:       "Valid",
			GpgKey:       "foo",
			LastSnapshot: &api.SnapshotResponse{},
		}
		repos[i] = repo
	}
	collection := api.RepositoryCollectionResponse{
		Data: repos,
	}
	params := fmt.Sprintf("?offset=%d&limit=%d", offset, limit)
	handler.SetCollectionResponseMetadata(&collection, getTestContext(params), int64(size))
	return collection
}

func getTestContext(params string) echo.Context {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/"+params, nil)
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec)
}
