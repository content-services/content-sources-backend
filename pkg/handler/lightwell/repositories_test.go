package lightwell

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/handler"
	"github.com/content-services/content-sources-backend/pkg/test"
	test_handler "github.com/content-services/content-sources-backend/pkg/test/handler"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type LightwellReposSuite struct {
	LightwellSuite
}

func TestLightwellReposSuite(t *testing.T) {
	suite.Run(t, new(LightwellReposSuite))
}

func createLightwellRepoCollection(size, limit, offset int) api.RepositoryCollectionResponse {
	repos := make([]api.RepositoryResponse, size)
	for i := 0; i < size; i++ {
		repo := api.RepositoryResponse{
			UUID:                 uuid.New().String(),
			Name:                 fmt.Sprintf("lightwell-repo-%d", i),
			URL:                  fmt.Sprintf("https://lightwell-repo-%d.example.com", i),
			DistributionVersions: []string{config.El9},
			DistributionArch:     config.X8664,
			AccountID:            test_handler.MockAccountNumber,
			OrgID:                test_handler.MockOrgId,
			Origin:               config.OriginLightwell,
			ContentType:          "maven",
		}
		repos[i] = repo
	}
	collection := api.RepositoryCollectionResponse{
		Data: repos,
	}
	return collection
}

func (s *LightwellReposSuite) TestListRepositories() {
	t := s.T()

	collection := createLightwellRepoCollection(5, 10, 0)
	paginationData := api.PaginationData{Limit: 10, Offset: handler.DefaultOffset}
	filterData := api.FilterData{Origin: config.OriginLightwell}

	s.reg.RepositoryConfig.WithContextMock().On("List", test.MockCtx(), test_handler.MockOrgId, paginationData, filterData).Return(collection, int64(5), nil).Once()
	s.reg.LightwellAdvisory.On("CountAdvisoriesByRepo", test.MockCtx(), uuid.MustParse(collection.Data[0].UUID)).Return(int64(3), nil)
	s.reg.LightwellAdvisory.On("CountAdvisoriesByRepo", test.MockCtx(), uuid.MustParse(collection.Data[1].UUID)).Return(int64(5), nil)
	s.reg.LightwellAdvisory.On("CountAdvisoriesByRepo", test.MockCtx(), uuid.MustParse(collection.Data[2].UUID)).Return(int64(0), nil)
	s.reg.LightwellAdvisory.On("CountAdvisoriesByRepo", test.MockCtx(), uuid.MustParse(collection.Data[3].UUID)).Return(int64(2), nil)
	s.reg.LightwellAdvisory.On("CountAdvisoriesByRepo", test.MockCtx(), uuid.MustParse(collection.Data[4].UUID)).Return(int64(1), nil)

	path := fmt.Sprintf("%s/repositories?limit=%d", LightwellAPIPath, 10)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := s.serveRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)

	response := api.RepositoryCollectionResponse{}
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	assert.Equal(t, 0, response.Meta.Offset)
	assert.Equal(t, int64(5), response.Meta.Count)
	assert.Equal(t, 10, response.Meta.Limit)
	assert.Equal(t, 5, len(response.Data))
	assert.Equal(t, collection.Data[0].Name, response.Data[0].Name)
	assert.Equal(t, collection.Data[0].URL, response.Data[0].URL)
	assert.Equal(t, config.OriginLightwell, response.Data[0].Origin)

	// Verify advisory counts are enriched
	assert.NotNil(t, response.Data[0].AdvisoryCount)
	assert.Equal(t, 3, *response.Data[0].AdvisoryCount)
	assert.NotNil(t, response.Data[1].AdvisoryCount)
	assert.Equal(t, 5, *response.Data[1].AdvisoryCount)
}

func (s *LightwellReposSuite) TestListNoRepositories() {
	t := s.T()

	collection := api.RepositoryCollectionResponse{Data: []api.RepositoryResponse{}}
	paginationData := api.PaginationData{Limit: handler.DefaultLimit, Offset: handler.DefaultOffset}
	filterData := api.FilterData{Origin: config.OriginLightwell}

	s.reg.RepositoryConfig.WithContextMock().On("List", test.MockCtx(), test_handler.MockOrgId, paginationData, filterData).Return(collection, int64(0), nil).Once()

	path := fmt.Sprintf("%s/repositories", LightwellAPIPath)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := s.serveRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)

	response := api.RepositoryCollectionResponse{}
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	assert.Equal(t, 0, response.Meta.Offset)
	assert.Equal(t, int64(0), response.Meta.Count)
	assert.Equal(t, 100, response.Meta.Limit)
	assert.Equal(t, 0, len(response.Data))
}

func (s *LightwellReposSuite) TestListPagedExtraRemaining() {
	t := s.T()

	collection := createLightwellRepoCollection(10, 10, 0)
	paginationData1 := api.PaginationData{Limit: 10, Offset: 0}
	paginationData2 := api.PaginationData{Limit: 10, Offset: 100}
	filterData := api.FilterData{Origin: config.OriginLightwell}

	s.reg.RepositoryConfig.WithContextMock().On("List", test.MockCtx(), test_handler.MockOrgId, paginationData1, filterData).Return(collection, int64(102), nil).Once()
	s.reg.RepositoryConfig.On("List", test.MockCtx(), test_handler.MockOrgId, paginationData2, filterData).Return(collection, int64(102), nil).Once()

	// Mock advisory counts for first page
	for i := 0; i < 10; i++ {
		s.reg.LightwellAdvisory.On("CountAdvisoriesByRepo", test.MockCtx(), uuid.MustParse(collection.Data[i].UUID)).Return(int64(i), nil).Maybe()
	}

	path := fmt.Sprintf("%s/repositories?limit=%d", LightwellAPIPath, 10)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := s.serveRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)

	response := api.RepositoryCollectionResponse{}
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	assert.Equal(t, 0, response.Meta.Offset)
	assert.Equal(t, 10, response.Meta.Limit)
	assert.Equal(t, int64(102), response.Meta.Count)
	assert.NotEmpty(t, response.Links.Last)

	// Fetch last page
	req = httptest.NewRequest(http.MethodGet, response.Links.Last, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))
	code, body, err = s.serveRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)

	response = api.RepositoryCollectionResponse{}
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
}

func (s *LightwellReposSuite) TestListPagedNoRemaining() {
	t := s.T()

	collection := createLightwellRepoCollection(10, 10, 0)
	paginationData1 := api.PaginationData{Limit: 10, Offset: 0}
	paginationData2 := api.PaginationData{Limit: 10, Offset: 90}
	filterData := api.FilterData{Origin: config.OriginLightwell}

	s.reg.RepositoryConfig.WithContextMock().On("List", test.MockCtx(), test_handler.MockOrgId, paginationData1, filterData).Return(collection, int64(100), nil).Once()
	s.reg.RepositoryConfig.On("List", test.MockCtx(), test_handler.MockOrgId, paginationData2, filterData).Return(collection, int64(100), nil).Once()

	// Mock advisory counts
	for i := 0; i < 10; i++ {
		s.reg.LightwellAdvisory.On("CountAdvisoriesByRepo", test.MockCtx(), uuid.MustParse(collection.Data[i].UUID)).Return(int64(0), nil).Maybe()
	}

	path := fmt.Sprintf("%s/repositories?limit=%d", LightwellAPIPath, 10)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := s.serveRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)

	response := api.RepositoryCollectionResponse{}
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	assert.Equal(t, 0, response.Meta.Offset)
	assert.Equal(t, 10, response.Meta.Limit)
	assert.Equal(t, int64(100), response.Meta.Count)
	assert.NotEmpty(t, response.Links.Last)

	// Fetch last page
	req = httptest.NewRequest(http.MethodGet, response.Links.Last, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))
	code, body, err = s.serveRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)

	response = api.RepositoryCollectionResponse{}
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
}

func (s *LightwellReposSuite) TestListWithFilters() {
	t := s.T()

	collection := createLightwellRepoCollection(2, 100, 0)
	paginationData := api.PaginationData{Limit: 100}
	filterData := api.FilterData{
		Origin:      config.OriginLightwell,
		ContentType: "maven",
		Name:        "test-repo",
	}

	s.reg.RepositoryConfig.WithContextMock().On("List", test.MockCtx(), test_handler.MockOrgId, paginationData, filterData).Return(collection, int64(2), nil).Once()

	// Mock advisory counts
	for i := 0; i < 2; i++ {
		s.reg.LightwellAdvisory.On("CountAdvisoriesByRepo", test.MockCtx(), uuid.MustParse(collection.Data[i].UUID)).Return(int64(0), nil)
	}

	path := fmt.Sprintf("%s/repositories?content_type=maven&name=test-repo", LightwellAPIPath)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := s.serveRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)
}

func (s *LightwellReposSuite) TestListWithSearch() {
	t := s.T()

	collection := createLightwellRepoCollection(1, 100, 0)
	paginationData := api.PaginationData{Limit: 100}
	filterData := api.FilterData{
		Origin: config.OriginLightwell,
		Search: "my-search-term",
	}

	s.reg.RepositoryConfig.WithContextMock().On("List", test.MockCtx(), test_handler.MockOrgId, paginationData, filterData).Return(collection, int64(1), nil).Once()
	s.reg.LightwellAdvisory.On("CountAdvisoriesByRepo", test.MockCtx(), uuid.MustParse(collection.Data[0].UUID)).Return(int64(0), nil)

	path := fmt.Sprintf("%s/repositories?search=my-search-term", LightwellAPIPath)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := s.serveRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)

	response := api.RepositoryCollectionResponse{}
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(response.Data))
}

func (s *LightwellReposSuite) TestListDaoError() {
	t := s.T()

	daoError := ce.DaoError{
		Message: "Database connection failed",
	}
	paginationData := api.PaginationData{Limit: handler.DefaultLimit}
	filterData := api.FilterData{Origin: config.OriginLightwell}

	s.reg.RepositoryConfig.WithContextMock().On("List", test.MockCtx(), test_handler.MockOrgId, paginationData, filterData).
		Return(api.RepositoryCollectionResponse{}, int64(0), &daoError).Once()

	path := fmt.Sprintf("%s/repositories", LightwellAPIPath)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := s.serveRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusInternalServerError, code)
}

func (s *LightwellReposSuite) TestAdvisoryCountEnrichment() {
	t := s.T()

	collection := createLightwellRepoCollection(3, 10, 0)
	paginationData := api.PaginationData{Limit: 10, Offset: handler.DefaultOffset}
	filterData := api.FilterData{Origin: config.OriginLightwell}

	s.reg.RepositoryConfig.WithContextMock().On("List", test.MockCtx(), test_handler.MockOrgId, paginationData, filterData).Return(collection, int64(3), nil).Once()

	// First repo has advisories
	s.reg.LightwellAdvisory.On("CountAdvisoriesByRepo", test.MockCtx(), uuid.MustParse(collection.Data[0].UUID)).Return(int64(10), nil)
	// Second repo has no advisories
	s.reg.LightwellAdvisory.On("CountAdvisoriesByRepo", test.MockCtx(), uuid.MustParse(collection.Data[1].UUID)).Return(int64(0), nil)
	// Third repo has error counting advisories (should be logged but not fail)
	s.reg.LightwellAdvisory.On("CountAdvisoriesByRepo", test.MockCtx(), uuid.MustParse(collection.Data[2].UUID)).Return(int64(0), &ce.DaoError{Message: "count failed"})

	path := fmt.Sprintf("%s/repositories?limit=%d", LightwellAPIPath, 10)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, body, err := s.serveRouter(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)

	response := api.RepositoryCollectionResponse{}
	err = json.Unmarshal(body, &response)
	assert.Nil(t, err)

	// First repo should have advisory count
	assert.NotNil(t, response.Data[0].AdvisoryCount)
	assert.Equal(t, 10, *response.Data[0].AdvisoryCount)

	// Second repo should have zero count
	assert.NotNil(t, response.Data[1].AdvisoryCount)
	assert.Equal(t, 0, *response.Data[1].AdvisoryCount)

	// Third repo should have nil count due to error
	assert.Nil(t, response.Data[2].AdvisoryCount)
}
