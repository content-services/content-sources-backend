// Code generated by mockery v2.46.2. DO NOT EDIT.

package pulp_client

import (
	context "context"

	zest "github.com/content-services/zest/release/v2024"
	mock "github.com/stretchr/testify/mock"
)

// MockPulpGlobalClient is an autogenerated mock type for the PulpGlobalClient type
type MockPulpGlobalClient struct {
	mock.Mock
}

// CancelTask provides a mock function with given fields: ctx, taskHref
func (_m *MockPulpGlobalClient) CancelTask(ctx context.Context, taskHref string) (zest.TaskResponse, error) {
	ret := _m.Called(ctx, taskHref)

	if len(ret) == 0 {
		panic("no return value specified for CancelTask")
	}

	var r0 zest.TaskResponse
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, string) (zest.TaskResponse, error)); ok {
		return rf(ctx, taskHref)
	}
	if rf, ok := ret.Get(0).(func(context.Context, string) zest.TaskResponse); ok {
		r0 = rf(ctx, taskHref)
	} else {
		r0 = ret.Get(0).(zest.TaskResponse)
	}

	if rf, ok := ret.Get(1).(func(context.Context, string) error); ok {
		r1 = rf(ctx, taskHref)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetContentPath provides a mock function with given fields: ctx
func (_m *MockPulpGlobalClient) GetContentPath(ctx context.Context) (string, error) {
	ret := _m.Called(ctx)

	if len(ret) == 0 {
		panic("no return value specified for GetContentPath")
	}

	var r0 string
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context) (string, error)); ok {
		return rf(ctx)
	}
	if rf, ok := ret.Get(0).(func(context.Context) string); ok {
		r0 = rf(ctx)
	} else {
		r0 = ret.Get(0).(string)
	}

	if rf, ok := ret.Get(1).(func(context.Context) error); ok {
		r1 = rf(ctx)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetTask provides a mock function with given fields: ctx, taskHref
func (_m *MockPulpGlobalClient) GetTask(ctx context.Context, taskHref string) (zest.TaskResponse, error) {
	ret := _m.Called(ctx, taskHref)

	if len(ret) == 0 {
		panic("no return value specified for GetTask")
	}

	var r0 zest.TaskResponse
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, string) (zest.TaskResponse, error)); ok {
		return rf(ctx, taskHref)
	}
	if rf, ok := ret.Get(0).(func(context.Context, string) zest.TaskResponse); ok {
		r0 = rf(ctx, taskHref)
	} else {
		r0 = ret.Get(0).(zest.TaskResponse)
	}

	if rf, ok := ret.Get(1).(func(context.Context, string) error); ok {
		r1 = rf(ctx, taskHref)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// LookupDomain provides a mock function with given fields: ctx, name
func (_m *MockPulpGlobalClient) LookupDomain(ctx context.Context, name string) (string, error) {
	ret := _m.Called(ctx, name)

	if len(ret) == 0 {
		panic("no return value specified for LookupDomain")
	}

	var r0 string
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, string) (string, error)); ok {
		return rf(ctx, name)
	}
	if rf, ok := ret.Get(0).(func(context.Context, string) string); ok {
		r0 = rf(ctx, name)
	} else {
		r0 = ret.Get(0).(string)
	}

	if rf, ok := ret.Get(1).(func(context.Context, string) error); ok {
		r1 = rf(ctx, name)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// LookupOrCreateDomain provides a mock function with given fields: ctx, name
func (_m *MockPulpGlobalClient) LookupOrCreateDomain(ctx context.Context, name string) (string, error) {
	ret := _m.Called(ctx, name)

	if len(ret) == 0 {
		panic("no return value specified for LookupOrCreateDomain")
	}

	var r0 string
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, string) (string, error)); ok {
		return rf(ctx, name)
	}
	if rf, ok := ret.Get(0).(func(context.Context, string) string); ok {
		r0 = rf(ctx, name)
	} else {
		r0 = ret.Get(0).(string)
	}

	if rf, ok := ret.Get(1).(func(context.Context, string) error); ok {
		r1 = rf(ctx, name)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// PollTask provides a mock function with given fields: ctx, taskHref
func (_m *MockPulpGlobalClient) PollTask(ctx context.Context, taskHref string) (*zest.TaskResponse, error) {
	ret := _m.Called(ctx, taskHref)

	if len(ret) == 0 {
		panic("no return value specified for PollTask")
	}

	var r0 *zest.TaskResponse
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, string) (*zest.TaskResponse, error)); ok {
		return rf(ctx, taskHref)
	}
	if rf, ok := ret.Get(0).(func(context.Context, string) *zest.TaskResponse); ok {
		r0 = rf(ctx, taskHref)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*zest.TaskResponse)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, string) error); ok {
		r1 = rf(ctx, taskHref)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// UpdateDomainIfNeeded provides a mock function with given fields: ctx, name
func (_m *MockPulpGlobalClient) UpdateDomainIfNeeded(ctx context.Context, name string) error {
	ret := _m.Called(ctx, name)

	if len(ret) == 0 {
		panic("no return value specified for UpdateDomainIfNeeded")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, string) error); ok {
		r0 = rf(ctx, name)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// NewMockPulpGlobalClient creates a new instance of MockPulpGlobalClient. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewMockPulpGlobalClient(t interface {
	mock.TestingT
	Cleanup(func())
}) *MockPulpGlobalClient {
	mock := &MockPulpGlobalClient{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
