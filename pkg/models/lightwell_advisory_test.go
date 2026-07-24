package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func validAdvisory() LightwellAdvisory {
	return LightwellAdvisory{
		RepoName:                    "lightwell/java/remediated",
		AdvisoryID:                  "ADV-001",
		RepositoryConfigurationUUID: "some-uuid",
		Checksum:                    "abc123",
	}
}

func TestLightwellAdvisoryValidate(t *testing.T) {
	la := validAdvisory()
	assert.NoError(t, la.validate())
}

func TestLightwellAdvisoryValidateRepoName(t *testing.T) {
	la := validAdvisory()
	la.RepoName = ""
	err := la.validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Repo name")
}

func TestLightwellAdvisoryValidateAdvisoryID(t *testing.T) {
	la := validAdvisory()
	la.AdvisoryID = ""
	err := la.validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Advisory ID")
}

func TestLightwellAdvisoryValidateRepoConfigUUID(t *testing.T) {
	la := validAdvisory()
	la.RepositoryConfigurationUUID = ""
	err := la.validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Repository configuration UUID")
}

func TestLightwellAdvisoryValidateChecksum(t *testing.T) {
	la := validAdvisory()
	la.Checksum = ""
	err := la.validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Checksum")
}
