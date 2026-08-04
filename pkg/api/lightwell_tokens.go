package api

import "time"

// LightwellTokenCreateRequest is the body for creating a Lightwell access token.
type LightwellTokenCreateRequest struct {
	Name      string     `json:"name"`                 // Human-readable token name
	UserID    *string    `json:"user_id,omitempty"`    // Associated user ID; defaults to the calling user
	ExpiresAt *time.Time `json:"expires_at,omitempty"` // Optional expiry; defaults to 365 days from creation (max 365 days)
}

// LightwellTokenResponse is metadata for a Lightwell access token (never includes the full secret after create).
type LightwellTokenResponse struct {
	UUID        string     `json:"uuid"`
	OrgID       string     `json:"org_id"`
	UserID      string     `json:"user_id"`
	Name        string     `json:"name"`
	TokenPrefix string     `json:"token_prefix"`
	Token       string     `json:"token,omitempty"` // Plaintext token; only present on create
	ExpiresAt   time.Time  `json:"expires_at"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// LightwellTokensResponse is a list of Lightwell access tokens.
type LightwellTokensResponse []LightwellTokenResponse

// LightwellTokenValidateRequest is the body for the internal token validate endpoint.
type LightwellTokenValidateRequest struct {
	Token string `json:"token"`          // Raw bearer token to validate
	Path  string `json:"path,omitempty"` // Optional request path for audit
}

// LightwellTokenValidateResponse is returned when a token is valid and entitled.
type LightwellTokenValidateResponse struct {
	OrgID     string `json:"org_id"`
	UserID    string `json:"user_id"`
	TokenUUID string `json:"token_uuid"`
}
