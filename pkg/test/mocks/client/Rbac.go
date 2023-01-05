// Code generated by mockery v2.14.0. DO NOT EDIT.

package client

import (
	client "github.com/content-services/content-sources-backend/pkg/client"
	mock "github.com/stretchr/testify/mock"
)

// Rbac is an autogenerated mock type for the Rbac type
type Rbac struct {
	mock.Mock
}

// Allowed provides a mock function with given fields: xrhid, resource, verb
func (_m *Rbac) Allowed(xrhid string, resource string, verb client.RbacVerb) (bool, error) {
	ret := _m.Called(xrhid, resource, verb)

	var r0 bool
	if rf, ok := ret.Get(0).(func(string, string, client.RbacVerb) bool); ok {
		r0 = rf(xrhid, resource, verb)
	} else {
		r0 = ret.Get(0).(bool)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string, string, client.RbacVerb) error); ok {
		r1 = rf(xrhid, resource, verb)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

type mockConstructorTestingTNewRbac interface {
	mock.TestingT
	Cleanup(func())
}

// NewRbac creates a new instance of Rbac. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
func NewRbac(t mockConstructorTestingTNewRbac) *Rbac {
	mock := &Rbac{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}