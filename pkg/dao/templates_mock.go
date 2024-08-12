// Code generated by mockery v2.43.0. DO NOT EDIT.

package dao

import (
	context "context"

	api "github.com/content-services/content-sources-backend/pkg/api"

	mock "github.com/stretchr/testify/mock"

	models "github.com/content-services/content-sources-backend/pkg/models"
)

// MockTemplateDao is an autogenerated mock type for the TemplateDao type
type MockTemplateDao struct {
	mock.Mock
}

// ClearDeletedAt provides a mock function with given fields: ctx, orgID, uuid
func (_m *MockTemplateDao) ClearDeletedAt(ctx context.Context, orgID string, uuid string) error {
	ret := _m.Called(ctx, orgID, uuid)

	if len(ret) == 0 {
		panic("no return value specified for ClearDeletedAt")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, string, string) error); ok {
		r0 = rf(ctx, orgID, uuid)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Create provides a mock function with given fields: ctx, templateRequest
func (_m *MockTemplateDao) Create(ctx context.Context, templateRequest api.TemplateRequest) (api.TemplateResponse, error) {
	ret := _m.Called(ctx, templateRequest)

	if len(ret) == 0 {
		panic("no return value specified for Create")
	}

	var r0 api.TemplateResponse
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, api.TemplateRequest) (api.TemplateResponse, error)); ok {
		return rf(ctx, templateRequest)
	}
	if rf, ok := ret.Get(0).(func(context.Context, api.TemplateRequest) api.TemplateResponse); ok {
		r0 = rf(ctx, templateRequest)
	} else {
		r0 = ret.Get(0).(api.TemplateResponse)
	}

	if rf, ok := ret.Get(1).(func(context.Context, api.TemplateRequest) error); ok {
		r1 = rf(ctx, templateRequest)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Delete provides a mock function with given fields: ctx, orgID, uuid
func (_m *MockTemplateDao) Delete(ctx context.Context, orgID string, uuid string) error {
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

// DeleteTemplateRepoConfigs provides a mock function with given fields: ctx, templateUUID, keepRepoConfigUUIDs
func (_m *MockTemplateDao) DeleteTemplateRepoConfigs(ctx context.Context, templateUUID string, keepRepoConfigUUIDs []string) error {
	ret := _m.Called(ctx, templateUUID, keepRepoConfigUUIDs)

	if len(ret) == 0 {
		panic("no return value specified for DeleteTemplateRepoConfigs")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, string, []string) error); ok {
		r0 = rf(ctx, templateUUID, keepRepoConfigUUIDs)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Fetch provides a mock function with given fields: ctx, orgID, uuid, includeSoftDel
func (_m *MockTemplateDao) Fetch(ctx context.Context, orgID string, uuid string, includeSoftDel bool) (api.TemplateResponse, error) {
	ret := _m.Called(ctx, orgID, uuid, includeSoftDel)

	if len(ret) == 0 {
		panic("no return value specified for Fetch")
	}

	var r0 api.TemplateResponse
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, string, string, bool) (api.TemplateResponse, error)); ok {
		return rf(ctx, orgID, uuid, includeSoftDel)
	}
	if rf, ok := ret.Get(0).(func(context.Context, string, string, bool) api.TemplateResponse); ok {
		r0 = rf(ctx, orgID, uuid, includeSoftDel)
	} else {
		r0 = ret.Get(0).(api.TemplateResponse)
	}

	if rf, ok := ret.Get(1).(func(context.Context, string, string, bool) error); ok {
		r1 = rf(ctx, orgID, uuid, includeSoftDel)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetDistributionHref provides a mock function with given fields: ctx, templateUUID, repoConfigUUID
func (_m *MockTemplateDao) GetDistributionHref(ctx context.Context, templateUUID string, repoConfigUUID string) (string, error) {
	ret := _m.Called(ctx, templateUUID, repoConfigUUID)

	if len(ret) == 0 {
		panic("no return value specified for GetDistributionHref")
	}

	var r0 string
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, string, string) (string, error)); ok {
		return rf(ctx, templateUUID, repoConfigUUID)
	}
	if rf, ok := ret.Get(0).(func(context.Context, string, string) string); ok {
		r0 = rf(ctx, templateUUID, repoConfigUUID)
	} else {
		r0 = ret.Get(0).(string)
	}

	if rf, ok := ret.Get(1).(func(context.Context, string, string) error); ok {
		r1 = rf(ctx, templateUUID, repoConfigUUID)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetRepoChanges provides a mock function with given fields: ctx, templateUUID, newRepoConfigUUIDs
func (_m *MockTemplateDao) GetRepoChanges(ctx context.Context, templateUUID string, newRepoConfigUUIDs []string) ([]string, []string, []string, []string, error) {
	ret := _m.Called(ctx, templateUUID, newRepoConfigUUIDs)

	if len(ret) == 0 {
		panic("no return value specified for GetRepoChanges")
	}

	var r0 []string
	var r1 []string
	var r2 []string
	var r3 []string
	var r4 error
	if rf, ok := ret.Get(0).(func(context.Context, string, []string) ([]string, []string, []string, []string, error)); ok {
		return rf(ctx, templateUUID, newRepoConfigUUIDs)
	}
	if rf, ok := ret.Get(0).(func(context.Context, string, []string) []string); ok {
		r0 = rf(ctx, templateUUID, newRepoConfigUUIDs)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]string)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, string, []string) []string); ok {
		r1 = rf(ctx, templateUUID, newRepoConfigUUIDs)
	} else {
		if ret.Get(1) != nil {
			r1 = ret.Get(1).([]string)
		}
	}

	if rf, ok := ret.Get(2).(func(context.Context, string, []string) []string); ok {
		r2 = rf(ctx, templateUUID, newRepoConfigUUIDs)
	} else {
		if ret.Get(2) != nil {
			r2 = ret.Get(2).([]string)
		}
	}

	if rf, ok := ret.Get(3).(func(context.Context, string, []string) []string); ok {
		r3 = rf(ctx, templateUUID, newRepoConfigUUIDs)
	} else {
		if ret.Get(3) != nil {
			r3 = ret.Get(3).([]string)
		}
	}

	if rf, ok := ret.Get(4).(func(context.Context, string, []string) error); ok {
		r4 = rf(ctx, templateUUID, newRepoConfigUUIDs)
	} else {
		r4 = ret.Error(4)
	}

	return r0, r1, r2, r3, r4
}

// InternalOnlyFetchByName provides a mock function with given fields: ctx, name
func (_m *MockTemplateDao) InternalOnlyFetchByName(ctx context.Context, name string) (models.Template, error) {
	ret := _m.Called(ctx, name)

	if len(ret) == 0 {
		panic("no return value specified for InternalOnlyFetchByName")
	}

	var r0 models.Template
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, string) (models.Template, error)); ok {
		return rf(ctx, name)
	}
	if rf, ok := ret.Get(0).(func(context.Context, string) models.Template); ok {
		r0 = rf(ctx, name)
	} else {
		r0 = ret.Get(0).(models.Template)
	}

	if rf, ok := ret.Get(1).(func(context.Context, string) error); ok {
		r1 = rf(ctx, name)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// List provides a mock function with given fields: ctx, orgID, paginationData, filterData
func (_m *MockTemplateDao) List(ctx context.Context, orgID string, paginationData api.PaginationData, filterData api.TemplateFilterData) (api.TemplateCollectionResponse, int64, error) {
	ret := _m.Called(ctx, orgID, paginationData, filterData)

	if len(ret) == 0 {
		panic("no return value specified for List")
	}

	var r0 api.TemplateCollectionResponse
	var r1 int64
	var r2 error
	if rf, ok := ret.Get(0).(func(context.Context, string, api.PaginationData, api.TemplateFilterData) (api.TemplateCollectionResponse, int64, error)); ok {
		return rf(ctx, orgID, paginationData, filterData)
	}
	if rf, ok := ret.Get(0).(func(context.Context, string, api.PaginationData, api.TemplateFilterData) api.TemplateCollectionResponse); ok {
		r0 = rf(ctx, orgID, paginationData, filterData)
	} else {
		r0 = ret.Get(0).(api.TemplateCollectionResponse)
	}

	if rf, ok := ret.Get(1).(func(context.Context, string, api.PaginationData, api.TemplateFilterData) int64); ok {
		r1 = rf(ctx, orgID, paginationData, filterData)
	} else {
		r1 = ret.Get(1).(int64)
	}

	if rf, ok := ret.Get(2).(func(context.Context, string, api.PaginationData, api.TemplateFilterData) error); ok {
		r2 = rf(ctx, orgID, paginationData, filterData)
	} else {
		r2 = ret.Error(2)
	}

	return r0, r1, r2
}

// SoftDelete provides a mock function with given fields: ctx, orgID, uuid
func (_m *MockTemplateDao) SoftDelete(ctx context.Context, orgID string, uuid string) error {
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

// Update provides a mock function with given fields: ctx, orgID, uuid, templParams
func (_m *MockTemplateDao) Update(ctx context.Context, orgID string, uuid string, templParams api.TemplateUpdateRequest) (api.TemplateResponse, error) {
	ret := _m.Called(ctx, orgID, uuid, templParams)

	if len(ret) == 0 {
		panic("no return value specified for Update")
	}

	var r0 api.TemplateResponse
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, string, string, api.TemplateUpdateRequest) (api.TemplateResponse, error)); ok {
		return rf(ctx, orgID, uuid, templParams)
	}
	if rf, ok := ret.Get(0).(func(context.Context, string, string, api.TemplateUpdateRequest) api.TemplateResponse); ok {
		r0 = rf(ctx, orgID, uuid, templParams)
	} else {
		r0 = ret.Get(0).(api.TemplateResponse)
	}

	if rf, ok := ret.Get(1).(func(context.Context, string, string, api.TemplateUpdateRequest) error); ok {
		r1 = rf(ctx, orgID, uuid, templParams)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// UpdateDistributionHrefs provides a mock function with given fields: ctx, templateUUID, repoUUIDs, repoDistributionMap
func (_m *MockTemplateDao) UpdateDistributionHrefs(ctx context.Context, templateUUID string, repoUUIDs []string, repoDistributionMap map[string]string) error {
	ret := _m.Called(ctx, templateUUID, repoUUIDs, repoDistributionMap)

	if len(ret) == 0 {
		panic("no return value specified for UpdateDistributionHrefs")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, string, []string, map[string]string) error); ok {
		r0 = rf(ctx, templateUUID, repoUUIDs, repoDistributionMap)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// NewMockTemplateDao creates a new instance of MockTemplateDao. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewMockTemplateDao(t interface {
	mock.TestingT
	Cleanup(func())
}) *MockTemplateDao {
	mock := &MockTemplateDao{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
