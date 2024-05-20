// Code generated by mockery v2.43.0. DO NOT EDIT.

package dao

import (
	context "context"

	mock "github.com/stretchr/testify/mock"
)

// MockMetricsDao is an autogenerated mock type for the MetricsDao type
type MockMetricsDao struct {
	mock.Mock
}

// OrganizationTotal provides a mock function with given fields: ctx
func (_m *MockMetricsDao) OrganizationTotal(ctx context.Context) int64 {
	ret := _m.Called(ctx)

	if len(ret) == 0 {
		panic("no return value specified for OrganizationTotal")
	}

	var r0 int64
	if rf, ok := ret.Get(0).(func(context.Context) int64); ok {
		r0 = rf(ctx)
	} else {
		r0 = ret.Get(0).(int64)
	}

	return r0
}

// PendingTasksAverageLatency provides a mock function with given fields: ctx
func (_m *MockMetricsDao) PendingTasksAverageLatency(ctx context.Context) float64 {
	ret := _m.Called(ctx)

	if len(ret) == 0 {
		panic("no return value specified for PendingTasksAverageLatency")
	}

	var r0 float64
	if rf, ok := ret.Get(0).(func(context.Context) float64); ok {
		r0 = rf(ctx)
	} else {
		r0 = ret.Get(0).(float64)
	}

	return r0
}

// PendingTasksCount provides a mock function with given fields: ctx
func (_m *MockMetricsDao) PendingTasksCount(ctx context.Context) int64 {
	ret := _m.Called(ctx)

	if len(ret) == 0 {
		panic("no return value specified for PendingTasksCount")
	}

	var r0 int64
	if rf, ok := ret.Get(0).(func(context.Context) int64); ok {
		r0 = rf(ctx)
	} else {
		r0 = ret.Get(0).(int64)
	}

	return r0
}

// PendingTasksOldestTask provides a mock function with given fields: ctx
func (_m *MockMetricsDao) PendingTasksOldestTask(ctx context.Context) float64 {
	ret := _m.Called(ctx)

	if len(ret) == 0 {
		panic("no return value specified for PendingTasksOldestTask")
	}

	var r0 float64
	if rf, ok := ret.Get(0).(func(context.Context) float64); ok {
		r0 = rf(ctx)
	} else {
		r0 = ret.Get(0).(float64)
	}

	return r0
}

// PublicRepositoriesFailedIntrospectionCount provides a mock function with given fields: ctx
func (_m *MockMetricsDao) PublicRepositoriesFailedIntrospectionCount(ctx context.Context) int {
	ret := _m.Called(ctx)

	if len(ret) == 0 {
		panic("no return value specified for PublicRepositoriesFailedIntrospectionCount")
	}

	var r0 int
	if rf, ok := ret.Get(0).(func(context.Context) int); ok {
		r0 = rf(ctx)
	} else {
		r0 = ret.Get(0).(int)
	}

	return r0
}

// RepositoriesCount provides a mock function with given fields: ctx
func (_m *MockMetricsDao) RepositoriesCount(ctx context.Context) int {
	ret := _m.Called(ctx)

	if len(ret) == 0 {
		panic("no return value specified for RepositoriesCount")
	}

	var r0 int
	if rf, ok := ret.Get(0).(func(context.Context) int); ok {
		r0 = rf(ctx)
	} else {
		r0 = ret.Get(0).(int)
	}

	return r0
}

// RepositoriesIntrospectionCount provides a mock function with given fields: ctx, hours, public
func (_m *MockMetricsDao) RepositoriesIntrospectionCount(ctx context.Context, hours int, public bool) IntrospectionCount {
	ret := _m.Called(ctx, hours, public)

	if len(ret) == 0 {
		panic("no return value specified for RepositoriesIntrospectionCount")
	}

	var r0 IntrospectionCount
	if rf, ok := ret.Get(0).(func(context.Context, int, bool) IntrospectionCount); ok {
		r0 = rf(ctx, hours, public)
	} else {
		r0 = ret.Get(0).(IntrospectionCount)
	}

	return r0
}

// RepositoryConfigsCount provides a mock function with given fields: ctx
func (_m *MockMetricsDao) RepositoryConfigsCount(ctx context.Context) int {
	ret := _m.Called(ctx)

	if len(ret) == 0 {
		panic("no return value specified for RepositoryConfigsCount")
	}

	var r0 int
	if rf, ok := ret.Get(0).(func(context.Context) int); ok {
		r0 = rf(ctx)
	} else {
		r0 = ret.Get(0).(int)
	}

	return r0
}

// NewMockMetricsDao creates a new instance of MockMetricsDao. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewMockMetricsDao(t interface {
	mock.TestingT
	Cleanup(func())
}) *MockMetricsDao {
	mock := &MockMetricsDao{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
