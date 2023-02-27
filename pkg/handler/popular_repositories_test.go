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
	test_handler "github.com/content-services/content-sources-backend/pkg/test/handler"
	"github.com/content-services/content-sources-backend/pkg/test/mocks"
	"github.com/labstack/echo/v4"
	"github.com/redhatinsights/platform-go-middlewares/identity"
	"github.com/stretchr/testify/assert"
)

var popularRepository = api.PopularRepositoryResponse{
	SuggestedName:        "EPEL 9 Everything x86_64",
	URL:                  "https://dl.fedoraproject.org/pub/epel/9/Everything/x86_64/",
	DistributionVersions: []string{"9"},
	DistributionArch:     "x86_64",
	GpgKey:               "-----BEGIN PGP PUBLIC KEY BLOCK-----\n\nmQINBGE3mOsBEACsU+XwJWDJVkItBaugXhXIIkb9oe+7aadELuVo0kBmc3HXt/Yp\nCJW9hHEiGZ6z2jwgPqyJjZhCvcAWvgzKcvqE+9i0NItV1rzfxrBe2BtUtZmVcuE6\n2b+SPfxQ2Hr8llaawRjt8BCFX/ZzM4/1Qk+EzlfTcEcpkMf6wdO7kD6ulBk/tbsW\nDHX2lNcxszTf+XP9HXHWJlA2xBfP+Dk4gl4DnO2Y1xR0OSywE/QtvEbN5cY94ieu\nn7CBy29AleMhmbnx9pw3NyxcFIAsEZHJoU4ZW9ulAJ/ogttSyAWeacW7eJGW31/Z\n39cS+I4KXJgeGRI20RmpqfH0tuT+X5Da59YpjYxkbhSK3HYBVnNPhoJFUc2j5iKy\nXLgkapu1xRnEJhw05kr4LCbud0NTvfecqSqa+59kuVc+zWmfTnGTYc0PXZ6Oa3rK\n44UOmE6eAT5zd/ToleDO0VesN+EO7CXfRsm7HWGpABF5wNK3vIEF2uRr2VJMvgqS\n9eNwhJyOzoca4xFSwCkc6dACGGkV+CqhufdFBhmcAsUotSxe3zmrBjqA0B/nxIvH\nDVgOAMnVCe+Lmv8T0mFgqZSJdIUdKjnOLu/GRFhjDKIak4jeMBMTYpVnU+HhMHLq\nuDiZkNEvEEGhBQmZuI8J55F/a6UURnxUwT3piyi3Pmr2IFD7ahBxPzOBCQARAQAB\ntCdGZWRvcmEgKGVwZWw5KSA8ZXBlbEBmZWRvcmFwcm9qZWN0Lm9yZz6JAk4EEwEI\nADgWIQT/itE0RZcQbs6BO5GKOHK/MihGfAUCYTeY6wIbDwULCQgHAgYVCgkICwIE\nFgIDAQIeAQIXgAAKCRCKOHK/MihGfFX/EACBPWv20+ttYu1A5WvtHJPzwbj0U4yF\n3zTQpBglQ2UfkRpYdipTlT3Ih6j5h2VmgRPtINCc/ZE28adrWpBoeFIS2YAKOCLC\nnZYtHl2nCoLq1U7FSttUGsZ/t8uGCBgnugTfnIYcmlP1jKKA6RJAclK89evDQX5n\nR9ZD+Cq3CBMlttvSTCht0qQVlwycedH8iWyYgP/mF0W35BIn7NuuZwWhgR00n/VG\n4nbKPOzTWbsP45awcmivdrS74P6mL84WfkghipdmcoyVb1B8ZP4Y/Ke0RXOnLhNe\nCfrXXvuW+Pvg2RTfwRDtehGQPAgXbmLmz2ZkV69RGIr54HJv84NDbqZovRTMr7gL\n9k3ciCzXCiYQgM8yAyGHV0KEhFSQ1HV7gMnt9UmxbxBE2pGU7vu3CwjYga5DpwU7\nw5wu1TmM5KgZtZvuWOTDnqDLf0cKoIbW8FeeCOn24elcj32bnQDuF9DPey1mqcvT\n/yEo/Ushyz6CVYxN8DGgcy2M9JOsnmjDx02h6qgWGWDuKgb9jZrvRedpAQCeemEd\nfhEs6ihqVxRFl16HxC4EVijybhAL76SsM2nbtIqW1apBQJQpXWtQwwdvgTVpdEtE\nr4ArVJYX5LrswnWEQMOelugUG6S3ZjMfcyOa/O0364iY73vyVgaYK+2XtT2usMux\nVL469Kj5m13T6w==\n=Mjs/\n-----END PGP PUBLIC KEY BLOCK-----",
	MetadataVerification: false,
}

func servePopularRepositoriesRouter(req *http.Request, mockDao *mocks.RepositoryConfigDao) (int, []byte, error) {
	router := echo.New()
	router.HTTPErrorHandler = config.CustomHTTPErrorHandler
	router.Use(middleware.WrapMiddlewareWithSkipper(identity.EnforceIdentity, middleware.SkipLiveness))
	pathPrefix := router.Group(fullRootPath())

	repoDao := dao.RepositoryConfigDao(mockDao)

	RegisterPopularRepositoriesRoutes(pathPrefix, &repoDao)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	response := rr.Result()
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	return response.StatusCode, body, err
}

func TestPopularRepos(t *testing.T) {
	mockDao := mocks.RepositoryConfigDao{}

	collection := createRepoCollection(0, 10, 0)
	paginationData := api.PaginationData{}
	mockDao.On("List", test_handler.MockOrgId, paginationData, api.FilterData{Search: "https://dl.fedoraproject.org/pub/epel/9/Everything/x86_64/"}).Return(collection, int64(0), nil)
	mockDao.On("List", test_handler.MockOrgId, paginationData, api.FilterData{Search: "https://dl.fedoraproject.org/pub/epel/8/Everything/x86_64/"}).Return(collection, int64(0), nil)
	mockDao.On("List", test_handler.MockOrgId, paginationData, api.FilterData{Search: "https://dl.fedoraproject.org/pub/epel/7/x86_64/"}).Return(collection, int64(0), nil)

	path := fmt.Sprintf("%s/popular_repositories/?limit=%d", fullRootPath(), 10)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := servePopularRepositoriesRouter(req, &mockDao)

	assert.Nil(t, err)

	response := api.PopularRepositoriesCollectionResponse{}
	err = json.Unmarshal(body, &response)

	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)
	mockDao.AssertExpectations(t)
	assert.Equal(t, 0, response.Meta.Offset)
	assert.Equal(t, int64(3), response.Meta.Count)
	assert.Equal(t, 10, response.Meta.Limit)
	assert.Equal(t, 3, len(response.Data))
	assert.Equal(t, response.Data[0].ExistingName, "")
}

func TestPopularReposSearchWithExisting(t *testing.T) {
	mockDao := mocks.RepositoryConfigDao{}
	magicalUUID := "Magical-UUID-21"
	existingName := "bestNameEver"
	collection := api.RepositoryCollectionResponse{Data: []api.RepositoryResponse{{UUID: magicalUUID, Name: existingName, URL: popularRepository.URL, DistributionVersions: popularRepository.DistributionVersions, DistributionArch: popularRepository.DistributionArch}}}
	paginationData := api.PaginationData{}
	mockDao.On("List", test_handler.MockOrgId, paginationData, api.FilterData{Search: popularRepository.URL}).Return(collection, int64(0), nil)

	path := fmt.Sprintf("%s/popular_repositories/?limit=%d&search=%s", fullRootPath(), 10, popularRepository.URL)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := servePopularRepositoriesRouter(req, &mockDao)

	assert.Nil(t, err)

	response := api.PopularRepositoriesCollectionResponse{}
	err = json.Unmarshal(body, &response)

	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)
	mockDao.AssertExpectations(t)
	assert.Equal(t, 0, response.Meta.Offset)
	assert.Equal(t, int64(1), response.Meta.Count)
	assert.Equal(t, 1, len(response.Data))
	assert.Equal(t, response.Data[0].URL, popularRepository.URL)
	assert.Equal(t, existingName, response.Data[0].ExistingName)
	assert.NotEqual(t, response.Data[0].ExistingName, response.Data[0].SuggestedName)
	assert.Equal(t, magicalUUID, response.Data[0].UUID)
}

func TestPopularReposSearchByURL(t *testing.T) {
	mockDao := mocks.RepositoryConfigDao{}

	collection := createRepoCollection(0, 10, 0)
	paginationData := api.PaginationData{}
	mockDao.On("List", test_handler.MockOrgId, paginationData, api.FilterData{Search: popularRepository.URL}).Return(collection, int64(0), nil)

	path := fmt.Sprintf("%s/popular_repositories/?limit=%d&search=%s", fullRootPath(), 10, popularRepository.URL)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := servePopularRepositoriesRouter(req, &mockDao)

	assert.Nil(t, err)

	response := api.PopularRepositoriesCollectionResponse{}
	err = json.Unmarshal(body, &response)

	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)
	mockDao.AssertExpectations(t)
	assert.Equal(t, 0, response.Meta.Offset)
	assert.Equal(t, int64(1), response.Meta.Count)
	assert.Equal(t, 10, response.Meta.Limit)
	assert.Equal(t, 1, len(response.Data))
	assert.Equal(t, response.Data[0].URL, popularRepository.URL)
}

func TestPopularReposSearchByName(t *testing.T) {
	mockDao := mocks.RepositoryConfigDao{}

	collection := createRepoCollection(0, 10, 0)
	paginationData := api.PaginationData{}
	mockDao.On("List", test_handler.MockOrgId, paginationData, api.FilterData{Search: popularRepository.URL}).Return(collection, int64(0), nil)

	path := fmt.Sprintf("%s/popular_repositories/?limit=%d&search=%s", fullRootPath(), 10, url.QueryEscape(popularRepository.SuggestedName))
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := servePopularRepositoriesRouter(req, &mockDao)

	assert.Nil(t, err)

	response := api.PopularRepositoriesCollectionResponse{}
	err = json.Unmarshal(body, &response)

	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)
	mockDao.AssertExpectations(t)
	assert.Equal(t, 0, response.Meta.Offset)
	assert.Equal(t, int64(1), response.Meta.Count)
	assert.Equal(t, 10, response.Meta.Limit)
	assert.Equal(t, 1, len(response.Data))
	assert.Equal(t, response.Data[0].SuggestedName, popularRepository.SuggestedName)
}
