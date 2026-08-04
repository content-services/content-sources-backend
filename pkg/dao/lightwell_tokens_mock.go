package dao

import (
	"context"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/stretchr/testify/mock"
)

// MockLightwellTokenDao is a testify mock for LightwellTokenDao.
type MockLightwellTokenDao struct {
	mock.Mock
}

func NewMockLightwellTokenDao(t interface {
	mock.TestingT
	Cleanup(func())
}) *MockLightwellTokenDao {
	m := &MockLightwellTokenDao{}
	m.Test(t)
	t.Cleanup(func() { m.AssertExpectations(t) })
	return m
}

func (m *MockLightwellTokenDao) Create(ctx context.Context, orgID string, userID string, name string, expiresAt *time.Time) (api.LightwellTokenResponse, error) {
	args := m.Called(ctx, orgID, userID, name, expiresAt)
	r0, _ := args.Get(0).(api.LightwellTokenResponse)
	return r0, args.Error(1)
}

func (m *MockLightwellTokenDao) ListByOrg(ctx context.Context, orgID string) (api.LightwellTokensResponse, error) {
	args := m.Called(ctx, orgID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	r0, _ := args.Get(0).(api.LightwellTokensResponse)
	return r0, args.Error(1)
}

func (m *MockLightwellTokenDao) Get(ctx context.Context, orgID string, uuid string) (api.LightwellTokenResponse, error) {
	args := m.Called(ctx, orgID, uuid)
	r0, _ := args.Get(0).(api.LightwellTokenResponse)
	return r0, args.Error(1)
}

func (m *MockLightwellTokenDao) Revoke(ctx context.Context, orgID string, uuid string) error {
	args := m.Called(ctx, orgID, uuid)
	return args.Error(0)
}

func (m *MockLightwellTokenDao) Validate(ctx context.Context, rawToken string) (api.LightwellTokenValidateResponse, error) {
	args := m.Called(ctx, rawToken)
	r0, _ := args.Get(0).(api.LightwellTokenValidateResponse)
	return r0, args.Error(1)
}
