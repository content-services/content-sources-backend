// Code generated by mockery. DO NOT EDIT.

package dao

import (
	context "context"

	api "github.com/content-services/content-sources-backend/pkg/api"

	mock "github.com/stretchr/testify/mock"

	models "github.com/content-services/content-sources-backend/pkg/models"
)

// MockRepositoryConfigDao is an autogenerated mock type for the RepositoryConfigDao type
type MockRepositoryConfigDao struct {
	mock.Mock
}

// BulkCreate provides a mock function with given fields: ctx, newRepositories
func (_m *MockRepositoryConfigDao) BulkCreate(ctx context.Context, newRepositories []api.RepositoryRequest) ([]api.RepositoryResponse, []error) {
	ret := _m.Called(ctx, newRepositories)

	if len(ret) == 0 {
		panic("no return value specified for BulkCreate")
	}

	var r0 []api.RepositoryResponse
	var r1 []error
	if rf, ok := ret.Get(0).(func(context.Context, []api.RepositoryRequest) ([]api.RepositoryResponse, []error)); ok {
		return rf(ctx, newRepositories)
	}
	if rf, ok := ret.Get(0).(func(context.Context, []api.RepositoryRequest) []api.RepositoryResponse); ok {
		r0 = rf(ctx, newRepositories)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]api.RepositoryResponse)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, []api.RepositoryRequest) []error); ok {
		r1 = rf(ctx, newRepositories)
	} else {
		if ret.Get(1) != nil {
			r1 = ret.Get(1).([]error)
		}
	}

	return r0, r1
}

// BulkDelete provides a mock function with given fields: ctx, orgID, uuids
func (_m *MockRepositoryConfigDao) BulkDelete(ctx context.Context, orgID string, uuids []string) []error {
	ret := _m.Called(ctx, orgID, uuids)

	if len(ret) == 0 {
		panic("no return value specified for BulkDelete")
	}

	var r0 []error
	if rf, ok := ret.Get(0).(func(context.Context, string, []string) []error); ok {
		r0 = rf(ctx, orgID, uuids)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]error)
		}
	}

	return r0
}

// BulkExport provides a mock function with given fields: ctx, orgID, reposToExport
func (_m *MockRepositoryConfigDao) BulkExport(ctx context.Context, orgID string, reposToExport api.RepositoryExportRequest) ([]api.RepositoryExportResponse, error) {
	ret := _m.Called(ctx, orgID, reposToExport)

	if len(ret) == 0 {
		panic("no return value specified for BulkExport")
	}

	var r0 []api.RepositoryExportResponse
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, string, api.RepositoryExportRequest) ([]api.RepositoryExportResponse, error)); ok {
		return rf(ctx, orgID, reposToExport)
	}
	if rf, ok := ret.Get(0).(func(context.Context, string, api.RepositoryExportRequest) []api.RepositoryExportResponse); ok {
		r0 = rf(ctx, orgID, reposToExport)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]api.RepositoryExportResponse)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, string, api.RepositoryExportRequest) error); ok {
		r1 = rf(ctx, orgID, reposToExport)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// BulkImport provides a mock function with given fields: ctx, reposToImport
func (_m *MockRepositoryConfigDao) BulkImport(ctx context.Context, reposToImport []api.RepositoryRequest) ([]api.RepositoryImportResponse, []error) {
	ret := _m.Called(ctx, reposToImport)

	if len(ret) == 0 {
		panic("no return value specified for BulkImport")
	}

	var r0 []api.RepositoryImportResponse
	var r1 []error
	if rf, ok := ret.Get(0).(func(context.Context, []api.RepositoryRequest) ([]api.RepositoryImportResponse, []error)); ok {
		return rf(ctx, reposToImport)
	}
	if rf, ok := ret.Get(0).(func(context.Context, []api.RepositoryRequest) []api.RepositoryImportResponse); ok {
		r0 = rf(ctx, reposToImport)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]api.RepositoryImportResponse)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, []api.RepositoryRequest) []error); ok {
		r1 = rf(ctx, reposToImport)
	} else {
		if ret.Get(1) != nil {
			r1 = ret.Get(1).([]error)
		}
	}

	return r0, r1
}

// Create provides a mock function with given fields: ctx, newRepo
func (_m *MockRepositoryConfigDao) Create(ctx context.Context, newRepo api.RepositoryRequest) (api.RepositoryResponse, error) {
	ret := _m.Called(ctx, newRepo)

	if len(ret) == 0 {
		panic("no return value specified for Create")
	}

	var r0 api.RepositoryResponse
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, api.RepositoryRequest) (api.RepositoryResponse, error)); ok {
		return rf(ctx, newRepo)
	}
	if rf, ok := ret.Get(0).(func(context.Context, api.RepositoryRequest) api.RepositoryResponse); ok {
		r0 = rf(ctx, newRepo)
	} else {
		r0 = ret.Get(0).(api.RepositoryResponse)
	}

	if rf, ok := ret.Get(1).(func(context.Context, api.RepositoryRequest) error); ok {
		r1 = rf(ctx, newRepo)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Delete provides a mock function with given fields: ctx, orgID, uuid
func (_m *MockRepositoryConfigDao) Delete(ctx context.Context, orgID string, uuid string) error {
	ret := _m.Called(ctx, orgID, uuid)

	if len(ret) == 0 {
		panic("no return value specified for Delete")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, string, string) error); ok {
		r0 = rf(ctx, orgID, uuid)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Fetch provides a mock function with given fields: ctx, orgID, uuid
func (_m *MockRepositoryConfigDao) Fetch(ctx context.Context, orgID string, uuid string) (api.RepositoryResponse, error) {
	ret := _m.Called(ctx, orgID, uuid)

	if len(ret) == 0 {
		panic("no return value specified for Fetch")
	}

	var r0 api.RepositoryResponse
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, string, string) (api.RepositoryResponse, error)); ok {
		return rf(ctx, orgID, uuid)
	}
	if rf, ok := ret.Get(0).(func(context.Context, string, string) api.RepositoryResponse); ok {
		r0 = rf(ctx, orgID, uuid)
	} else {
		r0 = ret.Get(0).(api.RepositoryResponse)
	}

	if rf, ok := ret.Get(1).(func(context.Context, string, string) error); ok {
		r1 = rf(ctx, orgID, uuid)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// FetchByRepoUuid provides a mock function with given fields: ctx, orgID, repoUuid
func (_m *MockRepositoryConfigDao) FetchByRepoUuid(ctx context.Context, orgID string, repoUuid string) (api.RepositoryResponse, error) {
	ret := _m.Called(ctx, orgID, repoUuid)

	if len(ret) == 0 {
		panic("no return value specified for FetchByRepoUuid")
	}

	var r0 api.RepositoryResponse
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, string, string) (api.RepositoryResponse, error)); ok {
		return rf(ctx, orgID, repoUuid)
	}
	if rf, ok := ret.Get(0).(func(context.Context, string, string) api.RepositoryResponse); ok {
		r0 = rf(ctx, orgID, repoUuid)
	} else {
		r0 = ret.Get(0).(api.RepositoryResponse)
	}

	if rf, ok := ret.Get(1).(func(context.Context, string, string) error); ok {
		r1 = rf(ctx, orgID, repoUuid)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// FetchWithoutOrgID provides a mock function with given fields: ctx, uuid, includeSoftDel
func (_m *MockRepositoryConfigDao) FetchWithoutOrgID(ctx context.Context, uuid string, includeSoftDel bool) (api.RepositoryResponse, error) {
	ret := _m.Called(ctx, uuid, includeSoftDel)

	if len(ret) == 0 {
		panic("no return value specified for FetchWithoutOrgID")
	}

	var r0 api.RepositoryResponse
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, string, bool) (api.RepositoryResponse, error)); ok {
		return rf(ctx, uuid, includeSoftDel)
	}
	if rf, ok := ret.Get(0).(func(context.Context, string, bool) api.RepositoryResponse); ok {
		r0 = rf(ctx, uuid, includeSoftDel)
	} else {
		r0 = ret.Get(0).(api.RepositoryResponse)
	}

	if rf, ok := ret.Get(1).(func(context.Context, string, bool) error); ok {
		r1 = rf(ctx, uuid, includeSoftDel)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// InternalOnly_FetchRepoConfigsForRepoUUID provides a mock function with given fields: ctx, uuid
func (_m *MockRepositoryConfigDao) InternalOnly_FetchRepoConfigsForRepoUUID(ctx context.Context, uuid string) []api.RepositoryResponse {
	ret := _m.Called(ctx, uuid)

	if len(ret) == 0 {
		panic("no return value specified for InternalOnly_FetchRepoConfigsForRepoUUID")
	}

	var r0 []api.RepositoryResponse
	if rf, ok := ret.Get(0).(func(context.Context, string) []api.RepositoryResponse); ok {
		r0 = rf(ctx, uuid)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]api.RepositoryResponse)
		}
	}

	return r0
}

// InternalOnly_IncrementFailedSnapshotCount provides a mock function with given fields: ctx, rcUuid
func (_m *MockRepositoryConfigDao) InternalOnly_IncrementFailedSnapshotCount(ctx context.Context, rcUuid string) error {
	ret := _m.Called(ctx, rcUuid)

	if len(ret) == 0 {
		panic("no return value specified for InternalOnly_IncrementFailedSnapshotCount")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, string) error); ok {
		r0 = rf(ctx, rcUuid)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// InternalOnly_ListReposToSnapshot provides a mock function with given fields: ctx, filter
func (_m *MockRepositoryConfigDao) InternalOnly_ListReposToSnapshot(ctx context.Context, filter *ListRepoFilter) ([]models.RepositoryConfiguration, error) {
	ret := _m.Called(ctx, filter)

	if len(ret) == 0 {
		panic("no return value specified for InternalOnly_ListReposToSnapshot")
	}

	var r0 []models.RepositoryConfiguration
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, *ListRepoFilter) ([]models.RepositoryConfiguration, error)); ok {
		return rf(ctx, filter)
	}
	if rf, ok := ret.Get(0).(func(context.Context, *ListRepoFilter) []models.RepositoryConfiguration); ok {
		r0 = rf(ctx, filter)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]models.RepositoryConfiguration)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, *ListRepoFilter) error); ok {
		r1 = rf(ctx, filter)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// InternalOnly_RefreshRedHatRepo provides a mock function with given fields: ctx, request, label, featureName
func (_m *MockRepositoryConfigDao) InternalOnly_RefreshRedHatRepo(ctx context.Context, request api.RepositoryRequest, label string, featureName string) (*api.RepositoryResponse, error) {
	ret := _m.Called(ctx, request, label, featureName)

	if len(ret) == 0 {
		panic("no return value specified for InternalOnly_RefreshRedHatRepo")
	}

	var r0 *api.RepositoryResponse
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, api.RepositoryRequest, string, string) (*api.RepositoryResponse, error)); ok {
		return rf(ctx, request, label, featureName)
	}
	if rf, ok := ret.Get(0).(func(context.Context, api.RepositoryRequest, string, string) *api.RepositoryResponse); ok {
		r0 = rf(ctx, request, label, featureName)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*api.RepositoryResponse)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, api.RepositoryRequest, string, string) error); ok {
		r1 = rf(ctx, request, label, featureName)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// InternalOnly_ResetFailedSnapshotCount provides a mock function with given fields: ctx, rcUuid
func (_m *MockRepositoryConfigDao) InternalOnly_ResetFailedSnapshotCount(ctx context.Context, rcUuid string) error {
	ret := _m.Called(ctx, rcUuid)

	if len(ret) == 0 {
		panic("no return value specified for InternalOnly_ResetFailedSnapshotCount")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, string) error); ok {
		r0 = rf(ctx, rcUuid)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// List provides a mock function with given fields: ctx, orgID, paginationData, filterData
func (_m *MockRepositoryConfigDao) List(ctx context.Context, orgID string, paginationData api.PaginationData, filterData api.FilterData) (api.RepositoryCollectionResponse, int64, error) {
	ret := _m.Called(ctx, orgID, paginationData, filterData)

	if len(ret) == 0 {
		panic("no return value specified for List")
	}

	var r0 api.RepositoryCollectionResponse
	var r1 int64
	var r2 error
	if rf, ok := ret.Get(0).(func(context.Context, string, api.PaginationData, api.FilterData) (api.RepositoryCollectionResponse, int64, error)); ok {
		return rf(ctx, orgID, paginationData, filterData)
	}
	if rf, ok := ret.Get(0).(func(context.Context, string, api.PaginationData, api.FilterData) api.RepositoryCollectionResponse); ok {
		r0 = rf(ctx, orgID, paginationData, filterData)
	} else {
		r0 = ret.Get(0).(api.RepositoryCollectionResponse)
	}

	if rf, ok := ret.Get(1).(func(context.Context, string, api.PaginationData, api.FilterData) int64); ok {
		r1 = rf(ctx, orgID, paginationData, filterData)
	} else {
		r1 = ret.Get(1).(int64)
	}

	if rf, ok := ret.Get(2).(func(context.Context, string, api.PaginationData, api.FilterData) error); ok {
		r2 = rf(ctx, orgID, paginationData, filterData)
	} else {
		r2 = ret.Error(2)
	}

	return r0, r1, r2
}

// ListReposWithOutdatedSnapshots provides a mock function with given fields: ctx, olderThanDays
func (_m *MockRepositoryConfigDao) ListReposWithOutdatedSnapshots(ctx context.Context, olderThanDays int) ([]models.RepositoryConfiguration, error) {
	ret := _m.Called(ctx, olderThanDays)

	if len(ret) == 0 {
		panic("no return value specified for ListReposWithOutdatedSnapshots")
	}

	var r0 []models.RepositoryConfiguration
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, int) ([]models.RepositoryConfiguration, error)); ok {
		return rf(ctx, olderThanDays)
	}
	if rf, ok := ret.Get(0).(func(context.Context, int) []models.RepositoryConfiguration); ok {
		r0 = rf(ctx, olderThanDays)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]models.RepositoryConfiguration)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, int) error); ok {
		r1 = rf(ctx, olderThanDays)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// SavePublicRepos provides a mock function with given fields: ctx, urls
func (_m *MockRepositoryConfigDao) SavePublicRepos(ctx context.Context, urls []string) error {
	ret := _m.Called(ctx, urls)

	if len(ret) == 0 {
		panic("no return value specified for SavePublicRepos")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, []string) error); ok {
		r0 = rf(ctx, urls)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SoftDelete provides a mock function with given fields: ctx, orgID, uuid
func (_m *MockRepositoryConfigDao) SoftDelete(ctx context.Context, orgID string, uuid string) error {
	ret := _m.Called(ctx, orgID, uuid)

	if len(ret) == 0 {
		panic("no return value specified for SoftDelete")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, string, string) error); ok {
		r0 = rf(ctx, orgID, uuid)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Update provides a mock function with given fields: ctx, orgID, uuid, repoParams
func (_m *MockRepositoryConfigDao) Update(ctx context.Context, orgID string, uuid string, repoParams api.RepositoryUpdateRequest) (bool, error) {
	ret := _m.Called(ctx, orgID, uuid, repoParams)

	if len(ret) == 0 {
		panic("no return value specified for Update")
	}

	var r0 bool
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, string, string, api.RepositoryUpdateRequest) (bool, error)); ok {
		return rf(ctx, orgID, uuid, repoParams)
	}
	if rf, ok := ret.Get(0).(func(context.Context, string, string, api.RepositoryUpdateRequest) bool); ok {
		r0 = rf(ctx, orgID, uuid, repoParams)
	} else {
		r0 = ret.Get(0).(bool)
	}

	if rf, ok := ret.Get(1).(func(context.Context, string, string, api.RepositoryUpdateRequest) error); ok {
		r1 = rf(ctx, orgID, uuid, repoParams)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// UpdateLastSnapshot provides a mock function with given fields: ctx, orgID, repoConfigUUID, snapUUID
func (_m *MockRepositoryConfigDao) UpdateLastSnapshot(ctx context.Context, orgID string, repoConfigUUID string, snapUUID string) error {
	ret := _m.Called(ctx, orgID, repoConfigUUID, snapUUID)

	if len(ret) == 0 {
		panic("no return value specified for UpdateLastSnapshot")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, string, string, string) error); ok {
		r0 = rf(ctx, orgID, repoConfigUUID, snapUUID)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// UpdateLastSnapshotTask provides a mock function with given fields: ctx, taskUUID, orgID, repoUUID
func (_m *MockRepositoryConfigDao) UpdateLastSnapshotTask(ctx context.Context, taskUUID string, orgID string, repoUUID string) error {
	ret := _m.Called(ctx, taskUUID, orgID, repoUUID)

	if len(ret) == 0 {
		panic("no return value specified for UpdateLastSnapshotTask")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, string, string, string) error); ok {
		r0 = rf(ctx, taskUUID, orgID, repoUUID)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// ValidateParameters provides a mock function with given fields: ctx, orgId, params, excludedUUIDS
func (_m *MockRepositoryConfigDao) ValidateParameters(ctx context.Context, orgId string, params api.RepositoryValidationRequest, excludedUUIDS []string) (api.RepositoryValidationResponse, error) {
	ret := _m.Called(ctx, orgId, params, excludedUUIDS)

	if len(ret) == 0 {
		panic("no return value specified for ValidateParameters")
	}

	var r0 api.RepositoryValidationResponse
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, string, api.RepositoryValidationRequest, []string) (api.RepositoryValidationResponse, error)); ok {
		return rf(ctx, orgId, params, excludedUUIDS)
	}
	if rf, ok := ret.Get(0).(func(context.Context, string, api.RepositoryValidationRequest, []string) api.RepositoryValidationResponse); ok {
		r0 = rf(ctx, orgId, params, excludedUUIDS)
	} else {
		r0 = ret.Get(0).(api.RepositoryValidationResponse)
	}

	if rf, ok := ret.Get(1).(func(context.Context, string, api.RepositoryValidationRequest, []string) error); ok {
		r1 = rf(ctx, orgId, params, excludedUUIDS)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// NewMockRepositoryConfigDao creates a new instance of MockRepositoryConfigDao. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewMockRepositoryConfigDao(t interface {
	mock.TestingT
	Cleanup(func())
}) *MockRepositoryConfigDao {
	mock := &MockRepositoryConfigDao{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
