// Code generated by mockery v2.32.0. DO NOT EDIT.

package pulp_client

import (
	zest "github.com/content-services/zest/release/v2024"
	mock "github.com/stretchr/testify/mock"
)

// MockPulpGlobalClient is an autogenerated mock type for the PulpGlobalClient type
type MockPulpGlobalClient struct {
	mock.Mock
}

// CancelTask provides a mock function with given fields: taskHref
func (_m *MockPulpGlobalClient) CancelTask(taskHref string) (zest.TaskResponse, error) {
	ret := _m.Called(taskHref)

	var r0 zest.TaskResponse
	var r1 error
	if rf, ok := ret.Get(0).(func(string) (zest.TaskResponse, error)); ok {
		return rf(taskHref)
	}
	if rf, ok := ret.Get(0).(func(string) zest.TaskResponse); ok {
		r0 = rf(taskHref)
	} else {
		r0 = ret.Get(0).(zest.TaskResponse)
	}

	if rf, ok := ret.Get(1).(func(string) error); ok {
		r1 = rf(taskHref)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetContentPath provides a mock function with given fields:
func (_m *MockPulpGlobalClient) GetContentPath() (string, error) {
	ret := _m.Called()

	var r0 string
	var r1 error
	if rf, ok := ret.Get(0).(func() (string, error)); ok {
		return rf()
	}
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	if rf, ok := ret.Get(1).(func() error); ok {
		r1 = rf()
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetTask provides a mock function with given fields: taskHref
func (_m *MockPulpGlobalClient) GetTask(taskHref string) (zest.TaskResponse, error) {
	ret := _m.Called(taskHref)

	var r0 zest.TaskResponse
	var r1 error
	if rf, ok := ret.Get(0).(func(string) (zest.TaskResponse, error)); ok {
		return rf(taskHref)
	}
	if rf, ok := ret.Get(0).(func(string) zest.TaskResponse); ok {
		r0 = rf(taskHref)
	} else {
		r0 = ret.Get(0).(zest.TaskResponse)
	}

	if rf, ok := ret.Get(1).(func(string) error); ok {
		r1 = rf(taskHref)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// LookupDomain provides a mock function with given fields: name
func (_m *MockPulpGlobalClient) LookupDomain(name string) (string, error) {
	ret := _m.Called(name)

	var r0 string
	var r1 error
	if rf, ok := ret.Get(0).(func(string) (string, error)); ok {
		return rf(name)
	}
	if rf, ok := ret.Get(0).(func(string) string); ok {
		r0 = rf(name)
	} else {
		r0 = ret.Get(0).(string)
	}

	if rf, ok := ret.Get(1).(func(string) error); ok {
		r1 = rf(name)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// LookupOrCreateDomain provides a mock function with given fields: name
func (_m *MockPulpGlobalClient) LookupOrCreateDomain(name string) (string, error) {
	ret := _m.Called(name)

	var r0 string
	var r1 error
	if rf, ok := ret.Get(0).(func(string) (string, error)); ok {
		return rf(name)
	}
	if rf, ok := ret.Get(0).(func(string) string); ok {
		r0 = rf(name)
	} else {
		r0 = ret.Get(0).(string)
	}

	if rf, ok := ret.Get(1).(func(string) error); ok {
		r1 = rf(name)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// PollTask provides a mock function with given fields: taskHref
func (_m *MockPulpGlobalClient) PollTask(taskHref string) (*zest.TaskResponse, error) {
	ret := _m.Called(taskHref)

	var r0 *zest.TaskResponse
	var r1 error
	if rf, ok := ret.Get(0).(func(string) (*zest.TaskResponse, error)); ok {
		return rf(taskHref)
	}
	if rf, ok := ret.Get(0).(func(string) *zest.TaskResponse); ok {
		r0 = rf(taskHref)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*zest.TaskResponse)
		}
	}

	if rf, ok := ret.Get(1).(func(string) error); ok {
		r1 = rf(taskHref)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// UpdateDomainIfNeeded provides a mock function with given fields: name
func (_m *MockPulpGlobalClient) UpdateDomainIfNeeded(name string) error {
	ret := _m.Called(name)

	var r0 error
	if rf, ok := ret.Get(0).(func(string) error); ok {
		r0 = rf(name)
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
