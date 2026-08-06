package models

import (
	"time"

	"gorm.io/gorm"
)

const TableNameLightwellAccessTokens = "lightwell_access_tokens"

// LightwellAccessToken stores a hashed Lightwell access token associated with an org and user.
type LightwellAccessToken struct {
	Base
	OrgID       string     `gorm:"not null"`
	UserID      string     `gorm:"not null"`
	Name        string     `gorm:"not null"`
	AccessLevel string     `gorm:"not null;default:validated"` // validated (full) or remediated-only
	TokenPrefix string     `gorm:"not null"`
	TokenHash   string     `gorm:"not null;uniqueIndex"`
	ExpiresAt   time.Time  `gorm:"not null"`
	RevokedAt   *time.Time `gorm:"default:null"`
	LastUsedAt  *time.Time `gorm:"default:null"`
}

func (t *LightwellAccessToken) TableName() string {
	return TableNameLightwellAccessTokens
}

func (t *LightwellAccessToken) BeforeCreate(tx *gorm.DB) error {
	if err := t.Base.BeforeCreate(tx); err != nil {
		return err
	}
	return t.validate()
}

func (t *LightwellAccessToken) BeforeUpdate(tx *gorm.DB) error {
	return t.validate()
}

func (t *LightwellAccessToken) validate() error {
	if t.OrgID == "" {
		return Error{Message: "Org ID cannot be blank.", Validation: true}
	}
	if t.UserID == "" {
		return Error{Message: "User ID cannot be blank.", Validation: true}
	}
	if t.Name == "" {
		return Error{Message: "Name cannot be blank.", Validation: true}
	}
	if t.AccessLevel != "validated" && t.AccessLevel != "remediated" {
		return Error{Message: "Access level must be validated or remediated.", Validation: true}
	}
	if t.TokenHash == "" {
		return Error{Message: "Token hash cannot be blank.", Validation: true}
	}
	if t.TokenPrefix == "" {
		return Error{Message: "Token prefix cannot be blank.", Validation: true}
	}
	if t.ExpiresAt.IsZero() {
		return Error{Message: "Expires at cannot be blank.", Validation: true}
	}
	return nil
}

func (t *LightwellAccessToken) IsRevoked() bool {
	return t.RevokedAt != nil
}

func (t *LightwellAccessToken) IsExpired(now time.Time) bool {
	return !t.ExpiresAt.After(now)
}
