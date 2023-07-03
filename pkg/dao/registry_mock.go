package dao

import (
	"testing"
)

type MockDaoRegistry struct {
	RepositoryConfig MockRepositoryConfigDao
	Rpm              MockRpmDao
	Repository       MockRepositoryDao
	Metrics          MockMetricsDao
	Snapshot         MockSnapshotDao
	TaskInfo         MockTaskInfoDao
	AdminTask        MockAdminTaskDao
	Domain           MockDomainDao
}

func (m *MockDaoRegistry) ToDaoRegistry() *DaoRegistry {
	r := DaoRegistry{
		RepositoryConfig: &m.RepositoryConfig,
		Rpm:              &m.Rpm,
		Repository:       &m.Repository,
		Metrics:          &m.Metrics,
		Snapshot:         &m.Snapshot,
		TaskInfo:         &m.TaskInfo,
		AdminTask:        &m.AdminTask,
		Domain:           &m.Domain,
	}
	return &r
}

func GetMockDaoRegistry(t *testing.T) *MockDaoRegistry {
	reg := MockDaoRegistry{
		RepositoryConfig: *NewMockRepositoryConfigDao(t),
		Rpm:              *NewMockRpmDao(t),
		Repository:       *NewMockRepositoryDao(t),
		Metrics:          *NewMockMetricsDao(t),
		Snapshot:         *NewMockSnapshotDao(t),
		TaskInfo:         *NewMockTaskInfoDao(t),
		AdminTask:        *NewMockAdminTaskDao(t),
		Domain:           *NewMockDomainDao(t),
	}
	return &reg
}
