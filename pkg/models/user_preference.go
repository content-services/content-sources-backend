package models

import (
	"fmt"

	"gorm.io/gorm"
)

const TableNameUserPreferences = "user_preferences"

// Valid user preference labels
const (
	UserPreferenceLightwellNotificationEnabled = "lightwell-notification-enabled"
	UserPreferenceLightwellNotificationMinimum = "lightwell-notification-minimum"
)

// ValidUserPreferenceLabels is the set of accepted preference labels
var ValidUserPreferenceLabels = map[string]bool{
	UserPreferenceLightwellNotificationEnabled: true,
	UserPreferenceLightwellNotificationMinimum: true,
}

// ValidUserPreferenceValues maps labels to their accepted values.
// Labels with an empty set accept any non-empty value.
var ValidUserPreferenceValues = map[string]map[string]bool{
	UserPreferenceLightwellNotificationEnabled: {
		"true":  true,
		"false": true,
	},
	UserPreferenceLightwellNotificationMinimum: {
		"critical": true,
		"high":     true,
		"medium":   true,
		"low":      true,
	},
}

// IsValidUserPreferenceLabel reports whether label is an accepted preference label
func IsValidUserPreferenceLabel(label string) bool {
	return ValidUserPreferenceLabels[label]
}

// ValidateUserPreferenceValue checks that value is valid for the given label
func ValidateUserPreferenceValue(label, value string) error {
	allowed, ok := ValidUserPreferenceValues[label]
	if !ok {
		return fmt.Errorf("invalid preference label: %s", label)
	}
	if len(allowed) == 0 {
		return nil
	}
	if !allowed[value] {
		return fmt.Errorf("invalid value %q for preference label %s", value, label)
	}
	return nil
}

// UserPreference stores a single user preference as a label/value pair
type UserPreference struct {
	Base
	OrgID  string `gorm:"not null"`
	UserID string `gorm:"not null"`
	Label  string `gorm:"not null"`
	Value  string `gorm:"not null"`
}

func (up *UserPreference) TableName() string {
	return TableNameUserPreferences
}

func (up *UserPreference) BeforeCreate(tx *gorm.DB) error {
	if err := up.Base.BeforeCreate(tx); err != nil {
		return err
	}
	return up.validate()
}

func (up *UserPreference) BeforeUpdate(tx *gorm.DB) error {
	return up.validate()
}

func (up *UserPreference) validate() error {
	if up.OrgID == "" {
		return Error{Message: "Org ID cannot be blank.", Validation: true}
	}
	if up.UserID == "" {
		return Error{Message: "User ID cannot be blank.", Validation: true}
	}
	if up.Label == "" {
		return Error{Message: "Label cannot be blank.", Validation: true}
	}
	if !IsValidUserPreferenceLabel(up.Label) {
		return Error{Message: fmt.Sprintf("Invalid preference label: %s", up.Label), Validation: true}
	}
	if up.Value == "" {
		return Error{Message: "Value cannot be blank.", Validation: true}
	}
	if err := ValidateUserPreferenceValue(up.Label, up.Value); err != nil {
		return Error{Message: err.Error(), Validation: true}
	}
	return nil
}
