// Code generated by mockery v2.35.4. DO NOT EDIT.

package candlepin_client

import mock "github.com/stretchr/testify/mock"

// MockCandlepinClient is an autogenerated mock type for the CandlepinClient type
type MockCandlepinClient struct {
	mock.Mock
}

// CreateOwner provides a mock function with given fields:
func (_m *MockCandlepinClient) CreateOwner() error {
	ret := _m.Called()

	var r0 error
	if rf, ok := ret.Get(0).(func() error); ok {
		r0 = rf()
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// ImportManifest provides a mock function with given fields: filename
func (_m *MockCandlepinClient) ImportManifest(filename string) error {
	ret := _m.Called(filename)

	var r0 error
	if rf, ok := ret.Get(0).(func(string) error); ok {
		r0 = rf(filename)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// ListContents provides a mock function with given fields: ownerKey
func (_m *MockCandlepinClient) ListContents(ownerKey string) ([]string, error) {
	ret := _m.Called(ownerKey)

	var r0 []string
	var r1 error
	if rf, ok := ret.Get(0).(func(string) ([]string, error)); ok {
		return rf(ownerKey)
	}
	if rf, ok := ret.Get(0).(func(string) []string); ok {
		r0 = rf(ownerKey)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]string)
		}
	}

	if rf, ok := ret.Get(1).(func(string) error); ok {
		r1 = rf(ownerKey)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// NewMockCandlepinClient creates a new instance of MockCandlepinClient. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewMockCandlepinClient(t interface {
	mock.TestingT
	Cleanup(func())
}) *MockCandlepinClient {
	mock := &MockCandlepinClient{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
