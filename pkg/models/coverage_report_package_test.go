package models

import (
	"testing"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type CoverageReportPackageSuite struct {
	*ModelsSuite
}

func TestCoverageReportPackageSuite(t *testing.T) {
	m := ModelsSuite{}
	r := CoverageReportPackageSuite{&m}
	suite.Run(t, &r)
}

func (s *CoverageReportPackageSuite) TestCoverageReportPackageCreate() {
	tx := s.tx

	report := CoverageReport{
		OrgID:  "org-1",
		Status: config.TaskStatusCompleted,
	}
	err := tx.Create(&report).Error
	assert.NoError(s.T(), err)

	pkg := CoverageReportPackage{
		CoverageReportUUID: report.UUID,
		Ecosystem:          "Java",
		Name:               "spring-core",
		Version:            "6.1.0",
		Namespace:          utils.Ptr("org.springframework"),
		MatchStatus:        CoverageMatchStatusExact,
	}
	err = tx.Create(&pkg).Error
	assert.NoError(s.T(), err)

	readPkg := CoverageReportPackage{}
	err = tx.Where("uuid = ?", pkg.UUID).First(&readPkg).Error
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), report.UUID, readPkg.CoverageReportUUID)
	assert.Equal(s.T(), pkg.Ecosystem, readPkg.Ecosystem)
	assert.Equal(s.T(), pkg.Name, readPkg.Name)
	assert.Equal(s.T(), pkg.Version, readPkg.Version)
	assert.Equal(s.T(), *pkg.Namespace, *readPkg.Namespace)
	assert.Equal(s.T(), pkg.MatchStatus, readPkg.MatchStatus)
}

func (s *CoverageReportPackageSuite) TestCoverageReportPackageCreateEmptyVersion() {
	tx := s.tx

	report := CoverageReport{
		OrgID:  "org-1",
		Status: config.TaskStatusCompleted,
	}
	err := tx.Create(&report).Error
	assert.NoError(s.T(), err)

	pkg := CoverageReportPackage{
		CoverageReportUUID: report.UUID,
		Ecosystem:          "Python",
		Name:               "flask",
		Version:            "",
		MatchStatus:        CoverageMatchStatusPartial,
	}
	err = tx.Create(&pkg).Error
	assert.NoError(s.T(), err)

	readPkg := CoverageReportPackage{}
	err = tx.Where("uuid = ?", pkg.UUID).First(&readPkg).Error
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), "", readPkg.Version)
}

func (s *CoverageReportPackageSuite) TestCoverageReportPackageValidations() {
	t := s.T()
	tx := s.tx

	report := CoverageReport{
		OrgID:  "org-1",
		Status: config.TaskStatusCompleted,
	}
	err := tx.Create(&report).Error
	assert.NoError(t, err)

	spName := "testcoveragereportpackagevalidations"
	testEcosystem := "Java"
	testName := "spring-core"
	testVersion := "6.1.0"
	testMatchStatus := CoverageMatchStatusExact

	testCases := []struct {
		given    CoverageReportPackage
		expected string
	}{
		{
			given: CoverageReportPackage{
				CoverageReportUUID: "",
				Ecosystem:          testEcosystem,
				Name:               testName,
				Version:            testVersion,
				MatchStatus:        testMatchStatus,
			},
			expected: "Coverage report UUID cannot be blank.",
		},
		{
			given: CoverageReportPackage{
				CoverageReportUUID: report.UUID,
				Ecosystem:          "",
				Name:               testName,
				Version:            testVersion,
				MatchStatus:        testMatchStatus,
			},
			expected: "Ecosystem cannot be blank.",
		},
		{
			given: CoverageReportPackage{
				CoverageReportUUID: report.UUID,
				Ecosystem:          testEcosystem,
				Name:               "",
				Version:            testVersion,
				MatchStatus:        testMatchStatus,
			},
			expected: "Package name cannot be blank.",
		},
		{
			given: CoverageReportPackage{
				CoverageReportUUID: report.UUID,
				Ecosystem:          testEcosystem,
				Name:               testName,
				Version:            testVersion,
				MatchStatus:        "",
			},
			expected: "Match status cannot be blank.",
		},
		{
			given: CoverageReportPackage{
				CoverageReportUUID: report.UUID,
				Ecosystem:          testEcosystem,
				Name:               testName,
				Version:            testVersion,
				MatchStatus:        "invalid",
			},
			expected: "Invalid package match status.",
		},
	}

	tx.SavePoint(spName)
	for _, item := range testCases {
		err := tx.Create(&item.given).Error
		assert.Error(t, err)
		if err != nil {
			assert.Equal(t, item.expected, err.Error())
		}
		tx.RollbackTo(spName)
	}
}
