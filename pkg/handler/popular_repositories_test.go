package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/middleware"
	"github.com/content-services/content-sources-backend/pkg/test"
	test_handler "github.com/content-services/content-sources-backend/pkg/test/handler"
	"github.com/labstack/echo/v4"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type PopularReposSuite struct {
	suite.Suite
	dao *dao.MockDaoRegistry
}

func TestPopularReposSuite(t *testing.T) {
	suite.Run(t, new(PopularReposSuite))
}
func (s *PopularReposSuite) SetupTest() {
	s.dao = dao.GetMockDaoRegistry(s.T())
}

var popularRepository = api.PopularRepositoryResponse{
	SuggestedName:        "EPEL 9 Everything x86_64",
	URL:                  "https://dl.fedoraproject.org/pub/epel/9/Everything/x86_64/",
	DistributionVersions: []string{"9"},
	DistributionArch:     "x86_64",
	GpgKey:               "-----BEGIN PGP PUBLIC KEY BLOCK-----\n\nmQINBGE3mOsBEACsU+XwJWDJVkItBaugXhXIIkb9oe+7aadELuVo0kBmc3HXt/Yp\nCJW9hHEiGZ6z2jwgPqyJjZhCvcAWvgzKcvqE+9i0NItV1rzfxrBe2BtUtZmVcuE6\n2b+SPfxQ2Hr8llaawRjt8BCFX/ZzM4/1Qk+EzlfTcEcpkMf6wdO7kD6ulBk/tbsW\nDHX2lNcxszTf+XP9HXHWJlA2xBfP+Dk4gl4DnO2Y1xR0OSywE/QtvEbN5cY94ieu\nn7CBy29AleMhmbnx9pw3NyxcFIAsEZHJoU4ZW9ulAJ/ogttSyAWeacW7eJGW31/Z\n39cS+I4KXJgeGRI20RmpqfH0tuT+X5Da59YpjYxkbhSK3HYBVnNPhoJFUc2j5iKy\nXLgkapu1xRnEJhw05kr4LCbud0NTvfecqSqa+59kuVc+zWmfTnGTYc0PXZ6Oa3rK\n44UOmE6eAT5zd/ToleDO0VesN+EO7CXfRsm7HWGpABF5wNK3vIEF2uRr2VJMvgqS\n9eNwhJyOzoca4xFSwCkc6dACGGkV+CqhufdFBhmcAsUotSxe3zmrBjqA0B/nxIvH\nDVgOAMnVCe+Lmv8T0mFgqZSJdIUdKjnOLu/GRFhjDKIak4jeMBMTYpVnU+HhMHLq\nuDiZkNEvEEGhBQmZuI8J55F/a6UURnxUwT3piyi3Pmr2IFD7ahBxPzOBCQARAQAB\ntCdGZWRvcmEgKGVwZWw5KSA8ZXBlbEBmZWRvcmFwcm9qZWN0Lm9yZz6JAk4EEwEI\nADgWIQT/itE0RZcQbs6BO5GKOHK/MihGfAUCYTeY6wIbDwULCQgHAgYVCgkICwIE\nFgIDAQIeAQIXgAAKCRCKOHK/MihGfFX/EACBPWv20+ttYu1A5WvtHJPzwbj0U4yF\n3zTQpBglQ2UfkRpYdipTlT3Ih6j5h2VmgRPtINCc/ZE28adrWpBoeFIS2YAKOCLC\nnZYtHl2nCoLq1U7FSttUGsZ/t8uGCBgnugTfnIYcmlP1jKKA6RJAclK89evDQX5n\nR9ZD+Cq3CBMlttvSTCht0qQVlwycedH8iWyYgP/mF0W35BIn7NuuZwWhgR00n/VG\n4nbKPOzTWbsP45awcmivdrS74P6mL84WfkghipdmcoyVb1B8ZP4Y/Ke0RXOnLhNe\nCfrXXvuW+Pvg2RTfwRDtehGQPAgXbmLmz2ZkV69RGIr54HJv84NDbqZovRTMr7gL\n9k3ciCzXCiYQgM8yAyGHV0KEhFSQ1HV7gMnt9UmxbxBE2pGU7vu3CwjYga5DpwU7\nw5wu1TmM5KgZtZvuWOTDnqDLf0cKoIbW8FeeCOn24elcj32bnQDuF9DPey1mqcvT\n/yEo/Ushyz6CVYxN8DGgcy2M9JOsnmjDx02h6qgWGWDuKgb9jZrvRedpAQCeemEd\nfhEs6ihqVxRFl16HxC4EVijybhAL76SsM2nbtIqW1apBQJQpXWtQwwdvgTVpdEtE\nr4ArVJYX5LrswnWEQMOelugUG6S3ZjMfcyOa/O0364iY73vyVgaYK+2XtT2usMux\nVL469Kj5m13T6w==\n=Mjs/\n-----END PGP PUBLIC KEY BLOCK-----",
	MetadataVerification: false,
}

func createPopularRepoList(size int) []api.PopularRepositoryResponse {
	repos := make([]api.PopularRepositoryResponse, size)
	for i := 0; i < size; i++ {
		repos[i] = api.PopularRepositoryResponse{
			UUID:                 fmt.Sprintf("%d", i),
			ExistingName:         fmt.Sprintf("repo_%d existing", i),
			SuggestedName:        fmt.Sprintf("repo_%d suggested", i),
			URL:                  fmt.Sprintf("http://repo-%d.com", i),
			DistributionVersions: []string{config.El7},
			DistributionArch:     config.X8664,
			GpgKey:               "foo",
			MetadataVerification: true,
		}
	}
	return repos
}

func (s *PopularReposSuite) servePopularRepositoriesRouter(req *http.Request) (int, []byte, error) {
	router := echo.New()
	router.HTTPErrorHandler = config.CustomHTTPErrorHandler
	router.Use(middleware.WrapMiddlewareWithSkipper(identity.EnforceIdentity, middleware.SkipAuth))
	pathPrefix := router.Group(api.FullRootPath())

	RegisterPopularRepositoriesRoutes(pathPrefix, s.dao.ToDaoRegistry())

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	response := rr.Result()
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	return response.StatusCode, body, err
}

func (s *PopularReposSuite) TestPopularRepos() {
	collection := createRepoCollection(0, 10, 0)
	paginationData := api.PaginationData{Limit: 1}
	s.dao.RepositoryConfig.WithContextMock().On("List", test.MockCtx(), test_handler.MockOrgId, paginationData, api.FilterData{Search: "https://dl.fedoraproject.org/pub/epel/9/Everything/x86_64/"}).Return(collection, int64(0), nil)
	s.dao.RepositoryConfig.On("List", test.MockCtx(), test_handler.MockOrgId, paginationData, api.FilterData{Search: "https://dl.fedoraproject.org/pub/epel/8/Everything/x86_64/"}).Return(collection, int64(0), nil)
	s.dao.RepositoryConfig.On("List", test.MockCtx(), test_handler.MockOrgId, paginationData, api.FilterData{Search: "https://dl.fedoraproject.org/pub/epel/7/x86_64/"}).Return(collection, int64(0), nil)

	path := fmt.Sprintf("%s/popular_repositories/?limit=%d", api.FullRootPath(), 10)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(s.T()))

	code, body, err := s.servePopularRepositoriesRouter(req)

	assert.Nil(s.T(), err)

	response := api.PopularRepositoriesCollectionResponse{}
	err = json.Unmarshal(body, &response)

	assert.Nil(s.T(), err)
	assert.Equal(s.T(), http.StatusOK, code)
	assert.Equal(s.T(), 0, response.Meta.Offset)
	assert.Equal(s.T(), int64(3), response.Meta.Count)
	assert.Equal(s.T(), 10, response.Meta.Limit)
	assert.Equal(s.T(), 3, len(response.Data))
	assert.Equal(s.T(), response.Data[0].ExistingName, "")
}

func (s *PopularReposSuite) TestPopularReposSearchWithExisting() {
	magicalUUID := "Magical-UUID-21"
	existingName := "bestNameEver"
	collection := api.RepositoryCollectionResponse{Data: []api.RepositoryResponse{{UUID: magicalUUID, Name: existingName, URL: popularRepository.URL, DistributionVersions: popularRepository.DistributionVersions, DistributionArch: popularRepository.DistributionArch}}}
	paginationData := api.PaginationData{Limit: 1}
	s.dao.RepositoryConfig.WithContextMock().On("List", test.MockCtx(), test_handler.MockOrgId, paginationData, api.FilterData{Search: popularRepository.URL}).Return(collection, int64(0), nil)

	path := fmt.Sprintf("%s/popular_repositories/?limit=%d&search=%s", api.FullRootPath(), 10, popularRepository.URL)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(s.T()))

	code, body, err := s.servePopularRepositoriesRouter(req)

	assert.Nil(s.T(), err)

	response := api.PopularRepositoriesCollectionResponse{}
	err = json.Unmarshal(body, &response)

	assert.Nil(s.T(), err)
	assert.Equal(s.T(), http.StatusOK, code)
	assert.Equal(s.T(), 0, response.Meta.Offset)
	assert.Equal(s.T(), int64(1), response.Meta.Count)
	assert.Equal(s.T(), 1, len(response.Data))
	assert.Equal(s.T(), response.Data[0].URL, popularRepository.URL)
	assert.Equal(s.T(), existingName, response.Data[0].ExistingName)
	assert.NotEqual(s.T(), response.Data[0].ExistingName, response.Data[0].SuggestedName)
	assert.Equal(s.T(), magicalUUID, response.Data[0].UUID)
}

func (s *PopularReposSuite) TestPopularReposSearchByURL() {
	collection := createRepoCollection(0, 10, 0)
	paginationData := api.PaginationData{Limit: 1}
	s.dao.RepositoryConfig.WithContextMock().On("List", test.MockCtx(), test_handler.MockOrgId, paginationData, api.FilterData{Search: popularRepository.URL}).Return(collection, int64(0), nil)
	path := fmt.Sprintf("%s/popular_repositories/?limit=%d&search=%s", api.FullRootPath(), 10, popularRepository.URL)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(s.T()))

	code, body, err := s.servePopularRepositoriesRouter(req)

	assert.Nil(s.T(), err)

	response := api.PopularRepositoriesCollectionResponse{}
	err = json.Unmarshal(body, &response)

	assert.Nil(s.T(), err)
	assert.Equal(s.T(), http.StatusOK, code)
	assert.Equal(s.T(), 0, response.Meta.Offset)
	assert.Equal(s.T(), int64(1), response.Meta.Count)
	assert.Equal(s.T(), 10, response.Meta.Limit)
	assert.Equal(s.T(), 1, len(response.Data))
	assert.Equal(s.T(), response.Data[0].URL, popularRepository.URL)
}

func (s *PopularReposSuite) TestPopularReposSearchByName() {
	collection := createRepoCollection(0, 10, 0)
	paginationData := api.PaginationData{Limit: 1}
	s.dao.RepositoryConfig.WithContextMock().On("List", test.MockCtx(), test_handler.MockOrgId, paginationData, api.FilterData{Search: popularRepository.URL}).Return(collection, int64(0), nil)

	path := fmt.Sprintf("%s/popular_repositories/?limit=%d&search=%s", api.FullRootPath(), 10, url.QueryEscape(popularRepository.SuggestedName))
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(s.T()))

	code, body, err := s.servePopularRepositoriesRouter(req)

	assert.Nil(s.T(), err)

	response := api.PopularRepositoriesCollectionResponse{}
	err = json.Unmarshal(body, &response)

	assert.Nil(s.T(), err)
	assert.Equal(s.T(), http.StatusOK, code)
	assert.Equal(s.T(), 0, response.Meta.Offset)
	assert.Equal(s.T(), int64(1), response.Meta.Count)
	assert.Equal(s.T(), 10, response.Meta.Limit)
	assert.Equal(s.T(), 1, len(response.Data))
	assert.Equal(s.T(), response.Data[0].SuggestedName, popularRepository.SuggestedName)
}

func (s *PopularReposSuite) TestPopularReposLimit() {
	collection := createPopularRepoList(20)
	response, total := filterPopularRepositories(collection, api.FilterData{}, api.PaginationData{Limit: 10})
	assert.Equal(s.T(), int64(20), total)
	assert.Equal(s.T(), 10, len(response.Data))
}

func (s *PopularReposSuite) TestPopularReposOutOfRangeOffset() {
	collection := createPopularRepoList(20)
	response, total := filterPopularRepositories(collection, api.FilterData{}, api.PaginationData{Offset: 20})
	assert.Equal(s.T(), int64(20), total)
	assert.Equal(s.T(), 0, len(response.Data))

	response, total = filterPopularRepositories(collection, api.FilterData{}, api.PaginationData{Offset: -10})
	assert.Equal(s.T(), int64(20), total)
	assert.Equal(s.T(), 0, len(response.Data))
}

func (s *PopularReposSuite) TestPopularReposPartialLastPage() {
	collection := createPopularRepoList(15)
	response, total := filterPopularRepositories(collection, api.FilterData{}, api.PaginationData{Offset: 10, Limit: 10})
	assert.Equal(s.T(), int64(15), total)
	assert.Equal(s.T(), 5, len(response.Data))
}

func (s *PopularReposSuite) TestPopularReposFullLastPage() {
	collection := createPopularRepoList(20)
	response1, total1 := filterPopularRepositories(collection, api.FilterData{}, api.PaginationData{Limit: 10, Offset: 0})
	assert.Equal(s.T(), int64(20), total1)
	assert.Equal(s.T(), 10, len(response1.Data))
	response2, total2 := filterPopularRepositories(collection, api.FilterData{}, api.PaginationData{Limit: 10, Offset: 10})
	assert.Equal(s.T(), int64(20), total2)
	assert.Equal(s.T(), 10, len(response2.Data))
	for i := 0; i < 10; i++ {
		assert.NotEqual(s.T(), response1.Data[i], response2.Data[i])
	}
}
