package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsValidUserPreferenceLabel(t *testing.T) {
	assert.True(t, IsValidUserPreferenceLabel(UserPreferenceLightwellNotificationEnabled))
	assert.True(t, IsValidUserPreferenceLabel(UserPreferenceLightwellNotificationMinimum))
	assert.True(t, IsValidUserPreferenceLabel(UserPreferenceLightwellTechPreviewAlertDismissed))
	assert.False(t, IsValidUserPreferenceLabel("unknown-label"))
}

func TestValidateUserPreferenceValue(t *testing.T) {
	assert.NoError(t, ValidateUserPreferenceValue(UserPreferenceLightwellNotificationEnabled, "true"))
	assert.NoError(t, ValidateUserPreferenceValue(UserPreferenceLightwellNotificationEnabled, "false"))
	assert.Error(t, ValidateUserPreferenceValue(UserPreferenceLightwellNotificationEnabled, "yes"))

	assert.NoError(t, ValidateUserPreferenceValue(UserPreferenceLightwellNotificationMinimum, "critical"))
	assert.NoError(t, ValidateUserPreferenceValue(UserPreferenceLightwellNotificationMinimum, "high"))
	assert.Error(t, ValidateUserPreferenceValue(UserPreferenceLightwellNotificationMinimum, "urgent"))

	assert.NoError(t, ValidateUserPreferenceValue(UserPreferenceLightwellTechPreviewAlertDismissed, "true"))
	assert.NoError(t, ValidateUserPreferenceValue(UserPreferenceLightwellTechPreviewAlertDismissed, "false"))
	assert.Error(t, ValidateUserPreferenceValue(UserPreferenceLightwellTechPreviewAlertDismissed, "dismissed"))
	assert.Error(t, ValidateUserPreferenceValue("unknown", "true"))
}

func TestUserPreferenceValidate(t *testing.T) {
	pref := UserPreference{
		OrgID:  "org",
		UserID: "user",
		Label:  UserPreferenceLightwellNotificationEnabled,
		Value:  "true",
	}
	assert.NoError(t, pref.validate())

	pref.Label = "bad"
	assert.Error(t, pref.validate())
}
