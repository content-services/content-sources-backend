package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func validAdvisoryNotification() LightwellAdvisoryNotification {
	return LightwellAdvisoryNotification{
		RepositoryConfigurationUUID: "some-uuid",
		AdvisoryID:                  "CVE-2026-1111",
		PackageName:                 "com.example:fake-lib",
	}
}

func TestLightwellAdvisoryNotificationValidate(t *testing.T) {
	lan := validAdvisoryNotification()
	assert.NoError(t, lan.validate())
}

func TestLightwellAdvisoryNotificationValidateRepoConfigUUID(t *testing.T) {
	lan := validAdvisoryNotification()
	lan.RepositoryConfigurationUUID = ""
	err := lan.validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Repository configuration UUID")
}

func TestLightwellAdvisoryNotificationValidateAdvisoryID(t *testing.T) {
	lan := validAdvisoryNotification()
	lan.AdvisoryID = ""
	err := lan.validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Advisory ID")
}

func TestLightwellAdvisoryNotificationTableName(t *testing.T) {
	lan := LightwellAdvisoryNotification{}
	assert.Equal(t, "lightwell_advisory_notifications", lan.TableName())
}
