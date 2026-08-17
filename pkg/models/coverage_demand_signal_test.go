package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type CoverageDemandSignalSuite struct {
	*ModelsSuite
}

func TestCoverageDemandSignalSuite(t *testing.T) {
	m := ModelsSuite{}
	r := CoverageDemandSignalSuite{&m}
	suite.Run(t, &r)
}

func (s *CoverageDemandSignalSuite) TestCoverageDemandSignalCreate() {
	tx := s.tx

	signal := CoverageDemandSignal{
		Ecosystem:   "Python",
		Name:        "custom-ml-lib",
		Version:     "0.1.0",
		MatchStatus: CoverageDemandMatchStatusNone,
		Source:      CoverageDemandSourceProspectDriven,
	}
	err := tx.Create(&signal).Error
	assert.NoError(s.T(), err)

	readSignal := CoverageDemandSignal{}
	err = tx.Where("uuid = ?", signal.UUID).First(&readSignal).Error
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), signal.Ecosystem, readSignal.Ecosystem)
	assert.Equal(s.T(), signal.Name, readSignal.Name)
	assert.Equal(s.T(), signal.Version, readSignal.Version)
	assert.Equal(s.T(), signal.MatchStatus, readSignal.MatchStatus)
	assert.Equal(s.T(), signal.Source, readSignal.Source)
}

func (s *CoverageDemandSignalSuite) TestCoverageDemandSignalValidations() {
	t := s.T()
	tx := s.tx

	spName := "testcoveragedemandsignalvalidations"
	testEcosystem := "Python"
	testName := "custom-ml-lib"
	testVersion := "0.1.0"
	testMatchStatus := CoverageDemandMatchStatusNone
	testSource := CoverageDemandSourceProspectDriven

	testCases := []struct {
		given    CoverageDemandSignal
		expected string
	}{
		{
			given: CoverageDemandSignal{
				Ecosystem:   "",
				Name:        testName,
				Version:     testVersion,
				MatchStatus: testMatchStatus,
				Source:      testSource,
			},
			expected: "Ecosystem cannot be blank.",
		},
		{
			given: CoverageDemandSignal{
				Ecosystem:   testEcosystem,
				Name:        "",
				Version:     testVersion,
				MatchStatus: testMatchStatus,
				Source:      testSource,
			},
			expected: "Package name cannot be blank.",
		},
		{
			given: CoverageDemandSignal{
				Ecosystem:   testEcosystem,
				Name:        testName,
				Version:     "",
				MatchStatus: testMatchStatus,
				Source:      testSource,
			},
			expected: "Version cannot be blank.",
		},
		{
			given: CoverageDemandSignal{
				Ecosystem:   testEcosystem,
				Name:        testName,
				Version:     testVersion,
				MatchStatus: "",
				Source:      testSource,
			},
			expected: "Match status cannot be blank.",
		},
		{
			given: CoverageDemandSignal{
				Ecosystem:   testEcosystem,
				Name:        testName,
				Version:     testVersion,
				MatchStatus: "exact",
				Source:      testSource,
			},
			expected: "Invalid demand signal match status.",
		},
		{
			given: CoverageDemandSignal{
				Ecosystem:   testEcosystem,
				Name:        testName,
				Version:     testVersion,
				MatchStatus: testMatchStatus,
				Source:      "",
			},
			expected: "Source cannot be blank.",
		},
		{
			given: CoverageDemandSignal{
				Ecosystem:   testEcosystem,
				Name:        testName,
				Version:     testVersion,
				MatchStatus: testMatchStatus,
				Source:      "other",
			},
			expected: "Invalid demand signal source.",
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
