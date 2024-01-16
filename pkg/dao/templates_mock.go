// Code generated by mockery v2.35.4. DO NOT EDIT.

package dao

import (
	api "github.com/content-services/content-sources-backend/pkg/api"
	mock "github.com/stretchr/testify/mock"
)

// MockTemplateDao is an autogenerated mock type for the TemplateDao type
type MockTemplateDao struct {
	mock.Mock
}

// Create provides a mock function with given fields: templateRequest
func (_m *MockTemplateDao) Create(templateRequest api.TemplateRequest) (api.TemplateResponse, error) {
	ret := _m.Called(templateRequest)

	var r0 api.TemplateResponse
	var r1 error
	if rf, ok := ret.Get(0).(func(api.TemplateRequest) (api.TemplateResponse, error)); ok {
		return rf(templateRequest)
	}
	if rf, ok := ret.Get(0).(func(api.TemplateRequest) api.TemplateResponse); ok {
		r0 = rf(templateRequest)
	} else {
		r0 = ret.Get(0).(api.TemplateResponse)
	}

	if rf, ok := ret.Get(1).(func(api.TemplateRequest) error); ok {
		r1 = rf(templateRequest)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Delete provides a mock function with given fields: orgID, uuid
func (_m *MockTemplateDao) Delete(orgID string, uuid string) error {
	ret := _m.Called(orgID, uuid)

	var r0 error
	if rf, ok := ret.Get(0).(func(string, string) error); ok {
		r0 = rf(orgID, uuid)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Fetch provides a mock function with given fields: orgID, uuid
func (_m *MockTemplateDao) Fetch(orgID string, uuid string) (api.TemplateResponse, error) {
	ret := _m.Called(orgID, uuid)

	var r0 api.TemplateResponse
	var r1 error
	if rf, ok := ret.Get(0).(func(string, string) (api.TemplateResponse, error)); ok {
		return rf(orgID, uuid)
	}
	if rf, ok := ret.Get(0).(func(string, string) api.TemplateResponse); ok {
		r0 = rf(orgID, uuid)
	} else {
		r0 = ret.Get(0).(api.TemplateResponse)
	}

	if rf, ok := ret.Get(1).(func(string, string) error); ok {
		r1 = rf(orgID, uuid)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// List provides a mock function with given fields: orgID, paginationData, filterData
func (_m *MockTemplateDao) List(orgID string, paginationData api.PaginationData, filterData api.TemplateFilterData) (api.TemplateCollectionResponse, int64, error) {
	ret := _m.Called(orgID, paginationData, filterData)

	var r0 api.TemplateCollectionResponse
	var r1 int64
	var r2 error
	if rf, ok := ret.Get(0).(func(string, api.PaginationData, api.TemplateFilterData) (api.TemplateCollectionResponse, int64, error)); ok {
		return rf(orgID, paginationData, filterData)
	}
	if rf, ok := ret.Get(0).(func(string, api.PaginationData, api.TemplateFilterData) api.TemplateCollectionResponse); ok {
		r0 = rf(orgID, paginationData, filterData)
	} else {
		r0 = ret.Get(0).(api.TemplateCollectionResponse)
	}

	if rf, ok := ret.Get(1).(func(string, api.PaginationData, api.TemplateFilterData) int64); ok {
		r1 = rf(orgID, paginationData, filterData)
	} else {
		r1 = ret.Get(1).(int64)
	}

	if rf, ok := ret.Get(2).(func(string, api.PaginationData, api.TemplateFilterData) error); ok {
		r2 = rf(orgID, paginationData, filterData)
	} else {
		r2 = ret.Error(2)
	}

	return r0, r1, r2
}

// SoftDelete provides a mock function with given fields: orgID, uuid
func (_m *MockTemplateDao) SoftDelete(orgID string, uuid string) error {
	ret := _m.Called(orgID, uuid)

	var r0 error
	if rf, ok := ret.Get(0).(func(string, string) error); ok {
		r0 = rf(orgID, uuid)
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
