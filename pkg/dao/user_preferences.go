package dao

import (
	"context"
	"fmt"

	"github.com/content-services/content-sources-backend/pkg/api"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type userPreferenceDaoImpl struct {
	db *gorm.DB
}

func (d userPreferenceDaoImpl) List(ctx context.Context, orgID string, userID string) (api.UserPreferencesResponse, error) {
	var prefs []models.UserPreference
	result := d.db.WithContext(ctx).
		Where("org_id = ? AND user_id = ?", orgID, userID).
		Order("label ASC").
		Find(&prefs)
	if result.Error != nil {
		return nil, userPreferenceDBErrorToApi(result.Error)
	}

	resp := make(api.UserPreferencesResponse, 0, len(prefs))
	for _, pref := range prefs {
		resp = append(resp, api.UserPreferenceResponse{
			Label: pref.Label,
			Value: pref.Value,
		})
	}
	return resp, nil
}

func (d userPreferenceDaoImpl) Set(ctx context.Context, orgID string, userID string, label string, value string) (api.UserPreferenceResponse, error) {
	// Validate before upsert: OnConflict DoUpdates bypasses GORM BeforeUpdate hooks.
	if !models.IsValidUserPreferenceLabel(label) {
		return api.UserPreferenceResponse{}, &ce.DaoError{
			BadValidation: true,
			Message:       fmt.Sprintf("invalid preference label: %s", label),
		}
	}
	if err := models.ValidateUserPreferenceValue(label, value); err != nil {
		return api.UserPreferenceResponse{}, &ce.DaoError{
			BadValidation: true,
			Message:       err.Error(),
		}
	}

	pref := models.UserPreference{
		OrgID:  orgID,
		UserID: userID,
		Label:  label,
		Value:  value,
	}

	result := d.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "org_id"}, {Name: "user_id"}, {Name: "label"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
	}).Create(&pref)
	if result.Error != nil {
		return api.UserPreferenceResponse{}, userPreferenceDBErrorToApi(result.Error)
	}

	return api.UserPreferenceResponse{
		Label: pref.Label,
		Value: pref.Value,
	}, nil
}

func (d userPreferenceDaoImpl) ListDistinctOrgsByPreference(ctx context.Context, label string, value string) ([]string, error) {
	var orgIDs []string
	result := d.db.WithContext(ctx).
		Model(&models.UserPreference{}).
		Where("label = ? AND value = ?", label, value).
		Distinct("org_id").
		Pluck("org_id", &orgIDs)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to list orgs by preference: %w", result.Error)
	}
	return orgIDs, nil
}

func userPreferenceDBErrorToApi(e error) *ce.DaoError {
	if dbError, ok := e.(models.Error); ok && dbError.Validation {
		return &ce.DaoError{BadValidation: true, Message: dbError.Message}
	}
	daoErr := ce.DaoError{Message: e.Error()}
	daoErr.Wrap(e)
	return &daoErr
}
