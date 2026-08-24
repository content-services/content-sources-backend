package dao

import (
	"testing"
)

type MockDaoRegistry struct {
	RepositoryConfig       MockRepositoryConfigDao
	Rpm                    MockRpmDao
	Repository             MockRepositoryDao
	Metrics                MockMetricsDao
	Snapshot               MockSnapshotDao
	TaskInfo               MockTaskInfoDao
	AdminTask              MockAdminTaskDao
	Domain                 MockDomainDao
	PackageGroup           MockPackageGroupDao
	Environment            MockEnvironmentDao
	Template               MockTemplateDao
	ModuleStream           MockModuleStreamDao
	MavenPackages          MockMavenPackagesDao
	LightwellAdvisory      MockLightwellAdvisoryDao
	LightwellVulnerability MockLightwellVulnerabilityDao
	UserPreference         MockUserPreferenceDao
	CoverageReport         MockCoverageReportDao
}

func (m *MockDaoRegistry) ToDaoRegistry() *DaoRegistry {
	r := DaoRegistry{
		RepositoryConfig:       &m.RepositoryConfig,
		Rpm:                    &m.Rpm,
		Repository:             &m.Repository,
		Metrics:                &m.Metrics,
		Snapshot:               &m.Snapshot,
		TaskInfo:               &m.TaskInfo,
		AdminTask:              &m.AdminTask,
		Domain:                 &m.Domain,
		PackageGroup:           &m.PackageGroup,
		Environment:            &m.Environment,
		Template:               &m.Template,
		ModuleStream:           &m.ModuleStream,
		MavenPackages:          &m.MavenPackages,
		LightwellAdvisory:      &m.LightwellAdvisory,
		LightwellVulnerability: &m.LightwellVulnerability,
		UserPreference:         &m.UserPreference,
		CoverageReport:         &m.CoverageReport,
	}
	return &r
}

func GetMockDaoRegistry(t *testing.T) *MockDaoRegistry {
	reg := MockDaoRegistry{
		RepositoryConfig:       *NewMockRepositoryConfigDao(t),
		Rpm:                    *NewMockRpmDao(t),
		Repository:             *NewMockRepositoryDao(t),
		Metrics:                *NewMockMetricsDao(t),
		Snapshot:               *NewMockSnapshotDao(t),
		TaskInfo:               *NewMockTaskInfoDao(t),
		AdminTask:              *NewMockAdminTaskDao(t),
		Domain:                 *NewMockDomainDao(t),
		PackageGroup:           *NewMockPackageGroupDao(t),
		Environment:            *NewMockEnvironmentDao(t),
		Template:               *NewMockTemplateDao(t),
		ModuleStream:           *NewMockModuleStreamDao(t),
		MavenPackages:          *NewMockMavenPackagesDao(t),
		LightwellAdvisory:      *NewMockLightwellAdvisoryDao(t),
		LightwellVulnerability: *NewMockLightwellVulnerabilityDao(t),
		UserPreference:         *NewMockUserPreferenceDao(t),
		CoverageReport:         *NewMockCoverageReportDao(t),
	}
	return &reg
}
