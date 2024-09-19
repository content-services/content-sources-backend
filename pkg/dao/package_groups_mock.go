// Code generated by mockery v2.46.0. DO NOT EDIT.

package dao

import (
	context "context"

	api "github.com/content-services/content-sources-backend/pkg/api"

	mock "github.com/stretchr/testify/mock"

	yum "github.com/content-services/yummy/pkg/yum"
)

// MockPackageGroupDao is an autogenerated mock type for the PackageGroupDao type
type MockPackageGroupDao struct {
	mock.Mock
}

// InsertForRepository provides a mock function with given fields: ctx, repoUuid, pkgGroups
func (_m *MockPackageGroupDao) InsertForRepository(ctx context.Context, repoUuid string, pkgGroups []yum.PackageGroup) (int64, error) {
	ret := _m.Called(ctx, repoUuid, pkgGroups)

	if len(ret) == 0 {
		panic("no return value specified for InsertForRepository")
	}

	var r0 int64
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, string, []yum.PackageGroup) (int64, error)); ok {
		return rf(ctx, repoUuid, pkgGroups)
	}
	if rf, ok := ret.Get(0).(func(context.Context, string, []yum.PackageGroup) int64); ok {
		r0 = rf(ctx, repoUuid, pkgGroups)
	} else {
		r0 = ret.Get(0).(int64)
	}

	if rf, ok := ret.Get(1).(func(context.Context, string, []yum.PackageGroup) error); ok {
		r1 = rf(ctx, repoUuid, pkgGroups)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// List provides a mock function with given fields: ctx, orgID, uuidRepo, limit, offset, search, sortBy
func (_m *MockPackageGroupDao) List(ctx context.Context, orgID string, uuidRepo string, limit int, offset int, search string, sortBy string) (api.RepositoryPackageGroupCollectionResponse, int64, error) {
	ret := _m.Called(ctx, orgID, uuidRepo, limit, offset, search, sortBy)

	if len(ret) == 0 {
		panic("no return value specified for List")
	}

	var r0 api.RepositoryPackageGroupCollectionResponse
	var r1 int64
	var r2 error
	if rf, ok := ret.Get(0).(func(context.Context, string, string, int, int, string, string) (api.RepositoryPackageGroupCollectionResponse, int64, error)); ok {
		return rf(ctx, orgID, uuidRepo, limit, offset, search, sortBy)
	}
	if rf, ok := ret.Get(0).(func(context.Context, string, string, int, int, string, string) api.RepositoryPackageGroupCollectionResponse); ok {
		r0 = rf(ctx, orgID, uuidRepo, limit, offset, search, sortBy)
	} else {
		r0 = ret.Get(0).(api.RepositoryPackageGroupCollectionResponse)
	}

	if rf, ok := ret.Get(1).(func(context.Context, string, string, int, int, string, string) int64); ok {
		r1 = rf(ctx, orgID, uuidRepo, limit, offset, search, sortBy)
	} else {
		r1 = ret.Get(1).(int64)
	}

	if rf, ok := ret.Get(2).(func(context.Context, string, string, int, int, string, string) error); ok {
		r2 = rf(ctx, orgID, uuidRepo, limit, offset, search, sortBy)
	} else {
		r2 = ret.Error(2)
	}

	return r0, r1, r2
}

// OrphanCleanup provides a mock function with given fields: ctx
func (_m *MockPackageGroupDao) OrphanCleanup(ctx context.Context) error {
	ret := _m.Called(ctx)

	if len(ret) == 0 {
		panic("no return value specified for OrphanCleanup")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context) error); ok {
		r0 = rf(ctx)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Search provides a mock function with given fields: ctx, orgID, request
func (_m *MockPackageGroupDao) Search(ctx context.Context, orgID string, request api.ContentUnitSearchRequest) ([]api.SearchPackageGroupResponse, error) {
	ret := _m.Called(ctx, orgID, request)

	if len(ret) == 0 {
		panic("no return value specified for Search")
	}

	var r0 []api.SearchPackageGroupResponse
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, string, api.ContentUnitSearchRequest) ([]api.SearchPackageGroupResponse, error)); ok {
		return rf(ctx, orgID, request)
	}
	if rf, ok := ret.Get(0).(func(context.Context, string, api.ContentUnitSearchRequest) []api.SearchPackageGroupResponse); ok {
		r0 = rf(ctx, orgID, request)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]api.SearchPackageGroupResponse)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, string, api.ContentUnitSearchRequest) error); ok {
		r1 = rf(ctx, orgID, request)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// SearchSnapshotPackageGroups provides a mock function with given fields: ctx, orgId, request
func (_m *MockPackageGroupDao) SearchSnapshotPackageGroups(ctx context.Context, orgId string, request api.SnapshotSearchRpmRequest) ([]api.SearchPackageGroupResponse, error) {
	ret := _m.Called(ctx, orgId, request)

	if len(ret) == 0 {
		panic("no return value specified for SearchSnapshotPackageGroups")
	}

	var r0 []api.SearchPackageGroupResponse
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, string, api.SnapshotSearchRpmRequest) ([]api.SearchPackageGroupResponse, error)); ok {
		return rf(ctx, orgId, request)
	}
	if rf, ok := ret.Get(0).(func(context.Context, string, api.SnapshotSearchRpmRequest) []api.SearchPackageGroupResponse); ok {
		r0 = rf(ctx, orgId, request)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]api.SearchPackageGroupResponse)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, string, api.SnapshotSearchRpmRequest) error); ok {
		r1 = rf(ctx, orgId, request)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// NewMockPackageGroupDao creates a new instance of MockPackageGroupDao. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewMockPackageGroupDao(t interface {
	mock.TestingT
	Cleanup(func())
}) *MockPackageGroupDao {
	mock := &MockPackageGroupDao{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
