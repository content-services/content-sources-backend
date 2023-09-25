// Code generated by mockery v2.20.0. DO NOT EDIT.

package dao

import (
	api "github.com/content-services/content-sources-backend/pkg/api"
	mock "github.com/stretchr/testify/mock"
)

// MockRepositoryDao is an autogenerated mock type for the RepositoryDao type
type MockRepositoryDao struct {
	mock.Mock
}

// FetchForUrl provides a mock function with given fields: url
func (_m *MockRepositoryDao) FetchForUrl(url string) (Repository, error) {
	ret := _m.Called(url)

	var r0 Repository
	var r1 error
	if rf, ok := ret.Get(0).(func(string) (Repository, error)); ok {
		return rf(url)
	}
	if rf, ok := ret.Get(0).(func(string) Repository); ok {
		r0 = rf(url)
	} else {
		r0 = ret.Get(0).(Repository)
	}

	if rf, ok := ret.Get(1).(func(string) error); ok {
		r1 = rf(url)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// FetchRepositoryRPMCount provides a mock function with given fields: repoUUID
func (_m *MockRepositoryDao) FetchRepositoryRPMCount(repoUUID string) (int, error) {
	ret := _m.Called(repoUUID)

	var r0 int
	var r1 error
	if rf, ok := ret.Get(0).(func(string) (int, error)); ok {
		return rf(repoUUID)
	}
	if rf, ok := ret.Get(0).(func(string) int); ok {
		r0 = rf(repoUUID)
	} else {
		r0 = ret.Get(0).(int)
	}

	if rf, ok := ret.Get(1).(func(string) error); ok {
		r1 = rf(repoUUID)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// List provides a mock function with given fields: ignoreFailed
func (_m *MockRepositoryDao) List(ignoreFailed bool) ([]Repository, error) {
	ret := _m.Called(ignoreFailed)

	var r0 []Repository
	var r1 error
	if rf, ok := ret.Get(0).(func(bool) ([]Repository, error)); ok {
		return rf(ignoreFailed)
	}
	if rf, ok := ret.Get(0).(func(bool) []Repository); ok {
		r0 = rf(ignoreFailed)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]Repository)
		}
	}

	if rf, ok := ret.Get(1).(func(bool) error); ok {
		r1 = rf(ignoreFailed)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// ListPublic provides a mock function with given fields: paginationData, _a1
func (_m *MockRepositoryDao) ListPublic(paginationData api.PaginationData, _a1 api.FilterData) (api.PublicRepositoryCollectionResponse, int64, error) {
	ret := _m.Called(paginationData, _a1)

	var r0 api.PublicRepositoryCollectionResponse
	var r1 int64
	var r2 error
	if rf, ok := ret.Get(0).(func(api.PaginationData, api.FilterData) (api.PublicRepositoryCollectionResponse, int64, error)); ok {
		return rf(paginationData, _a1)
	}
	if rf, ok := ret.Get(0).(func(api.PaginationData, api.FilterData) api.PublicRepositoryCollectionResponse); ok {
		r0 = rf(paginationData, _a1)
	} else {
		r0 = ret.Get(0).(api.PublicRepositoryCollectionResponse)
	}

	if rf, ok := ret.Get(1).(func(api.PaginationData, api.FilterData) int64); ok {
		r1 = rf(paginationData, _a1)
	} else {
		r1 = ret.Get(1).(int64)
	}

	if rf, ok := ret.Get(2).(func(api.PaginationData, api.FilterData) error); ok {
		r2 = rf(paginationData, _a1)
	} else {
		r2 = ret.Error(2)
	}

	return r0, r1, r2
}

// OrphanCleanup provides a mock function with given fields:
func (_m *MockRepositoryDao) OrphanCleanup() error {
	ret := _m.Called()

	var r0 error
	if rf, ok := ret.Get(0).(func() error); ok {
		r0 = rf()
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Update provides a mock function with given fields: repo
func (_m *MockRepositoryDao) Update(repo RepositoryUpdate) error {
	ret := _m.Called(repo)

	var r0 error
	if rf, ok := ret.Get(0).(func(RepositoryUpdate) error); ok {
		r0 = rf(repo)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

type mockConstructorTestingTNewMockRepositoryDao interface {
	mock.TestingT
	Cleanup(func())
}

// NewMockRepositoryDao creates a new instance of MockRepositoryDao. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
func NewMockRepositoryDao(t mockConstructorTestingTNewMockRepositoryDao) *MockRepositoryDao {
	mock := &MockRepositoryDao{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
