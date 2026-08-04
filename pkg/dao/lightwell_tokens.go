package dao

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/models"
	"gorm.io/gorm"
)

const (
	lightwellTokenPrefix      = "lw_"
	lightwellTokenPrefixLen   = 11 // "lw_" + 8 chars of secret
	lightwellTokenSecretBytes = 32
	lightwellTokenDefaultTTL  = 365 * 24 * time.Hour
	lightwellTokenMaxTTL      = 365 * 24 * time.Hour
	lightwellTokenNameMaxLen  = 255
)

type lightwellTokenDaoImpl struct {
	db *gorm.DB
}

func (d lightwellTokenDaoImpl) Create(ctx context.Context, orgID string, userID string, name string, expiresAt *time.Time) (api.LightwellTokenResponse, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return api.LightwellTokenResponse{}, &ce.DaoError{BadValidation: true, Message: "name is required"}
	}
	if len(name) > lightwellTokenNameMaxLen {
		return api.LightwellTokenResponse{}, &ce.DaoError{BadValidation: true, Message: "name is too long"}
	}
	if userID == "" {
		return api.LightwellTokenResponse{}, &ce.DaoError{BadValidation: true, Message: "user_id is required"}
	}
	if orgID == "" {
		return api.LightwellTokenResponse{}, &ce.DaoError{BadValidation: true, Message: "org_id is required"}
	}

	pepper := config.Get().Options.LightwellTokenPepper
	if pepper == "" {
		return api.LightwellTokenResponse{}, &ce.DaoError{Message: "lightwell token pepper is not configured"}
	}

	now := time.Now().UTC()
	expiry := now.Add(lightwellTokenDefaultTTL)
	if expiresAt != nil {
		expiry = expiresAt.UTC()
		if !expiry.After(now) {
			return api.LightwellTokenResponse{}, &ce.DaoError{BadValidation: true, Message: "expires_at must be in the future"}
		}
		if expiry.Sub(now) > lightwellTokenMaxTTL {
			return api.LightwellTokenResponse{}, &ce.DaoError{BadValidation: true, Message: "expires_at cannot be more than 365 days from now"}
		}
	}

	plaintext, err := generateLightwellToken()
	if err != nil {
		return api.LightwellTokenResponse{}, &ce.DaoError{Message: fmt.Sprintf("failed to generate token: %v", err)}
	}

	token := models.LightwellAccessToken{
		OrgID:       orgID,
		UserID:      userID,
		Name:        name,
		TokenPrefix: plaintext[:lightwellTokenPrefixLen],
		TokenHash:   hashLightwellToken(pepper, plaintext),
		ExpiresAt:   expiry,
	}

	result := d.db.WithContext(ctx).Create(&token)
	if result.Error != nil {
		return api.LightwellTokenResponse{}, lightwellTokenDBErrorToApi(result.Error)
	}

	resp := modelToLightwellTokenResponse(token)
	resp.Token = plaintext
	return resp, nil
}

func (d lightwellTokenDaoImpl) ListByOrg(ctx context.Context, orgID string) (api.LightwellTokensResponse, error) {
	var tokens []models.LightwellAccessToken
	result := d.db.WithContext(ctx).
		Where("org_id = ?", orgID).
		Order("created_at DESC").
		Find(&tokens)
	if result.Error != nil {
		return nil, lightwellTokenDBErrorToApi(result.Error)
	}

	resp := make(api.LightwellTokensResponse, 0, len(tokens))
	for _, token := range tokens {
		resp = append(resp, modelToLightwellTokenResponse(token))
	}
	return resp, nil
}

func (d lightwellTokenDaoImpl) Get(ctx context.Context, orgID string, uuid string) (api.LightwellTokenResponse, error) {
	var token models.LightwellAccessToken
	result := d.db.WithContext(ctx).
		Where("org_id = ? AND uuid = ?", orgID, uuid).
		First(&token)
	if result.Error != nil {
		return api.LightwellTokenResponse{}, lightwellTokenDBErrorToApi(result.Error)
	}
	return modelToLightwellTokenResponse(token), nil
}

func (d lightwellTokenDaoImpl) Revoke(ctx context.Context, orgID string, uuid string) error {
	now := time.Now().UTC()
	result := d.db.WithContext(ctx).
		Session(&gorm.Session{SkipHooks: true}).
		Model(&models.LightwellAccessToken{}).
		Where("org_id = ? AND uuid = ? AND revoked_at IS NULL", orgID, uuid).
		Updates(map[string]any{
			"revoked_at": now,
			"updated_at": now,
		})
	if result.Error != nil {
		return lightwellTokenDBErrorToApi(result.Error)
	}
	if result.RowsAffected == 0 {
		// Distinguish missing vs already revoked
		var existing models.LightwellAccessToken
		find := d.db.WithContext(ctx).Where("org_id = ? AND uuid = ?", orgID, uuid).First(&existing)
		if find.Error != nil {
			return lightwellTokenDBErrorToApi(find.Error)
		}
		return nil
	}
	return nil
}

func (d lightwellTokenDaoImpl) Validate(ctx context.Context, rawToken string) (api.LightwellTokenValidateResponse, error) {
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" {
		return api.LightwellTokenValidateResponse{}, &ce.DaoError{NotFound: true, Message: "token not found"}
	}

	pepper := config.Get().Options.LightwellTokenPepper
	if pepper == "" {
		return api.LightwellTokenValidateResponse{}, &ce.DaoError{Message: "lightwell token pepper is not configured"}
	}

	var token models.LightwellAccessToken
	result := d.db.WithContext(ctx).
		Where("token_hash = ?", hashLightwellToken(pepper, rawToken)).
		First(&token)
	if result.Error != nil {
		return api.LightwellTokenValidateResponse{}, lightwellTokenDBErrorToApi(result.Error)
	}

	now := time.Now().UTC()
	if token.IsRevoked() {
		return api.LightwellTokenValidateResponse{}, &ce.DaoError{NotFound: true, Message: "token has been revoked"}
	}
	if token.IsExpired(now) {
		return api.LightwellTokenValidateResponse{}, &ce.DaoError{NotFound: true, Message: "token has expired"}
	}

	_ = d.db.WithContext(ctx).
		Session(&gorm.Session{SkipHooks: true}).
		Model(&models.LightwellAccessToken{}).
		Where("uuid = ?", token.UUID).
		Updates(map[string]any{
			"last_used_at": now,
			"updated_at":   now,
		})

	return api.LightwellTokenValidateResponse{
		OrgID:     token.OrgID,
		UserID:    token.UserID,
		TokenUUID: token.UUID,
	}, nil
}

func generateLightwellToken() (string, error) {
	buf := make([]byte, lightwellTokenSecretBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	secret := base64.RawURLEncoding.EncodeToString(buf)
	return lightwellTokenPrefix + secret, nil
}

func hashLightwellToken(pepper, plaintext string) string {
	mac := hmac.New(sha256.New, []byte(pepper))
	_, _ = mac.Write([]byte(plaintext))
	return hex.EncodeToString(mac.Sum(nil))
}

func modelToLightwellTokenResponse(token models.LightwellAccessToken) api.LightwellTokenResponse {
	return api.LightwellTokenResponse{
		UUID:        token.UUID,
		OrgID:       token.OrgID,
		UserID:      token.UserID,
		Name:        token.Name,
		TokenPrefix: token.TokenPrefix,
		ExpiresAt:   token.ExpiresAt.UTC(),
		RevokedAt:   token.RevokedAt,
		LastUsedAt:  token.LastUsedAt,
		CreatedAt:   token.CreatedAt.UTC(),
	}
}

func lightwellTokenDBErrorToApi(e error) *ce.DaoError {
	if e == nil {
		return nil
	}
	if dbError, ok := e.(models.Error); ok && dbError.Validation {
		return &ce.DaoError{BadValidation: true, Message: dbError.Message}
	}
	daoErr := ce.DaoError{Message: e.Error()}
	daoErr.Wrap(e)
	return &daoErr
}
