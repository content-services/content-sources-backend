// Code generated by mockery v2.42.3. DO NOT EDIT.

package candlepin_client

import (
	context "context"

	caliri "github.com/content-services/caliri/release/v4"

	mock "github.com/stretchr/testify/mock"
)

// MockCandlepinClient is an autogenerated mock type for the CandlepinClient type
type MockCandlepinClient struct {
	mock.Mock
}

// AddContentBatchToProduct provides a mock function with given fields: ctx, ownerKey, contentIDs
func (_m *MockCandlepinClient) AddContentBatchToProduct(ctx context.Context, ownerKey string, contentIDs []string) error {
	ret := _m.Called(ctx, ownerKey, contentIDs)

	if len(ret) == 0 {
		panic("no return value specified for CreateOwner")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, string, []string) error); ok {
		r0 = rf(ctx, ownerKey, contentIDs)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// CreateContent provides a mock function with given fields: ctx, ownerKey, content
func (_m *MockCandlepinClient) CreateContent(ctx context.Context, ownerKey string, content caliri.ContentDTO) error {
	ret := _m.Called(ctx, ownerKey, content)

	if len(ret) == 0 {
		panic("no return value specified for ImportManifest")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, string, caliri.ContentDTO) error); ok {
		r0 = rf(ctx, ownerKey, content)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// CreateContentBatch provides a mock function with given fields: ctx, ownerKey, content
func (_m *MockCandlepinClient) CreateContentBatch(ctx context.Context, ownerKey string, content []caliri.ContentDTO) error {
	ret := _m.Called(ctx, ownerKey, content)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, string, []caliri.ContentDTO) error); ok {
		r0 = rf(ctx, ownerKey, content)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// CreateEnvironment provides a mock function with given fields: ctx, ownerKey, name, id, prefix
func (_m *MockCandlepinClient) CreateEnvironment(ctx context.Context, ownerKey string, name string, id string, prefix string) (*caliri.EnvironmentDTO, error) {
	ret := _m.Called(ctx, ownerKey, name, id, prefix)

	var r0 *caliri.EnvironmentDTO
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, string, string, string, string) (*caliri.EnvironmentDTO, error)); ok {
		return rf(ctx, ownerKey, name, id, prefix)
	}
	if rf, ok := ret.Get(0).(func(context.Context, string, string, string, string) *caliri.EnvironmentDTO); ok {
		r0 = rf(ctx, ownerKey, name, id, prefix)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*caliri.EnvironmentDTO)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, string, string, string, string) error); ok {
		r1 = rf(ctx, ownerKey, name, id, prefix)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// CreateOwner provides a mock function with given fields: ctx
func (_m *MockCandlepinClient) CreateOwner(ctx context.Context) error {
	ret := _m.Called(ctx)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context) error); ok {
		r0 = rf(ctx)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// CreatePool provides a mock function with given fields: ctx, ownerKey
func (_m *MockCandlepinClient) CreatePool(ctx context.Context, ownerKey string) (string, error) {
	ret := _m.Called(ctx, ownerKey)

	var r0 string
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, string) (string, error)); ok {
		return rf(ctx, ownerKey)
	}
	if rf, ok := ret.Get(0).(func(context.Context, string) string); ok {
		r0 = rf(ctx, ownerKey)
	} else {
		r0 = ret.Get(0).(string)
	}

	if rf, ok := ret.Get(1).(func(context.Context, string) error); ok {
		r1 = rf(ctx, ownerKey)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// CreateProduct provides a mock function with given fields: ctx, ownerKey
func (_m *MockCandlepinClient) CreateProduct(ctx context.Context, ownerKey string) error {
	ret := _m.Called(ctx, ownerKey)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, string) error); ok {
		r0 = rf(ctx, ownerKey)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// DemoteContentFromEnvironment provides a mock function with given fields: ctx, envID, contentIDs
func (_m *MockCandlepinClient) DemoteContentFromEnvironment(ctx context.Context, envID string, contentIDs []string) error {
	ret := _m.Called(ctx, envID, contentIDs)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, string, []string) error); ok {
		r0 = rf(ctx, envID, contentIDs)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// FetchEnvironment provides a mock function with given fields: ctx, envID
func (_m *MockCandlepinClient) FetchEnvironment(ctx context.Context, envID string) (*caliri.EnvironmentDTO, error) {
	ret := _m.Called(ctx, envID)

	var r0 *caliri.EnvironmentDTO
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, string) (*caliri.EnvironmentDTO, error)); ok {
		return rf(ctx, envID)
	}
	if rf, ok := ret.Get(0).(func(context.Context, string) *caliri.EnvironmentDTO); ok {
		r0 = rf(ctx, envID)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*caliri.EnvironmentDTO)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, string) error); ok {
		r1 = rf(ctx, envID)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// FetchPool provides a mock function with given fields: ctx, ownerKey
func (_m *MockCandlepinClient) FetchPool(ctx context.Context, ownerKey string) (*caliri.PoolDTO, error) {
	ret := _m.Called(ctx, ownerKey)

	var r0 *caliri.PoolDTO
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, string) (*caliri.PoolDTO, error)); ok {
		return rf(ctx, ownerKey)
	}
	if rf, ok := ret.Get(0).(func(context.Context, string) *caliri.PoolDTO); ok {
		r0 = rf(ctx, ownerKey)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*caliri.PoolDTO)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, string) error); ok {
		r1 = rf(ctx, ownerKey)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// FetchProduct provides a mock function with given fields: ctx, ownerKey, productID
func (_m *MockCandlepinClient) FetchProduct(ctx context.Context, ownerKey string, productID string) (*caliri.ProductDTO, error) {
	ret := _m.Called(ctx, ownerKey, productID)

	var r0 *caliri.ProductDTO
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, string, string) (*caliri.ProductDTO, error)); ok {
		return rf(ctx, ownerKey, productID)
	}
	if rf, ok := ret.Get(0).(func(context.Context, string, string) *caliri.ProductDTO); ok {
		r0 = rf(ctx, ownerKey, productID)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*caliri.ProductDTO)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, string, string) error); ok {
		r1 = rf(ctx, ownerKey, productID)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// ImportManifest provides a mock function with given fields: ctx, filename
func (_m *MockCandlepinClient) ImportManifest(ctx context.Context, filename string) error {
	ret := _m.Called(ctx, filename)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, string) error); ok {
		r0 = rf(ctx, filename)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// ListContents provides a mock function with given fields: ctx, ownerKey
func (_m *MockCandlepinClient) ListContents(ctx context.Context, ownerKey string) ([]string, error) {
	ret := _m.Called(ctx, ownerKey)

	if len(ret) == 0 {
		panic("no return value specified for ListContents")
	}

	var r0 []string
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, string) ([]string, error)); ok {
		return rf(ctx, ownerKey)
	}
	if rf, ok := ret.Get(0).(func(context.Context, string) []string); ok {
		r0 = rf(ctx, ownerKey)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]string)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, string) error); ok {
		r1 = rf(ctx, ownerKey)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// PromoteContentToEnvironment provides a mock function with given fields: ctx, envID, contentIDs
func (_m *MockCandlepinClient) PromoteContentToEnvironment(ctx context.Context, envID string, contentIDs []string) error {
	ret := _m.Called(ctx, envID, contentIDs)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, string, []string) error); ok {
		r0 = rf(ctx, envID, contentIDs)
	} else {
		r0 = ret.Error(0)
	}

	return r0
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
