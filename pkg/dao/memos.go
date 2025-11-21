package dao

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

type memoDaoImpl struct {
	db *gorm.DB
}

func (mDao memoDaoImpl) Read(ctx context.Context, key string) (*models.Memo, error) {
	var memo models.Memo
	result := mDao.db.WithContext(ctx).Where("key = ?", key).First(&memo)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, result.Error
	}
	return &memo, nil
}

func (mDao memoDaoImpl) Write(ctx context.Context, key string, memoData json.RawMessage) (*models.Memo, error) {
	memo := models.Memo{
		Key:  key,
		Memo: memoData,
	}

	// Use upsert to create or update the memo
	result := mDao.db.WithContext(ctx).Where("key = ?", key).Assign(models.Memo{Memo: memoData}).FirstOrCreate(&memo)
	if result.Error != nil {
		return nil, result.Error
	}

	return &memo, nil
}

type pulpLogDateMemo struct {
	Date string `json:"date"`
}

// GetLastSuccessfulPulpLogDate retrieves the last successful pulp log date from memo
// If no memo exists, return an error
func (mDao memoDaoImpl) GetLastSuccessfulPulpLogDate(ctx context.Context) (time.Time, error) {
	memo, err := mDao.Read(ctx, config.MemoPulpLastSuccessfulPulpLogParse)

	now := time.Now().UTC()
	if err != nil {
		log.Error().Err(err).Msgf("failed to read pulp last successful pulp log")
		return now, err
	}
	if memo == nil {
		return now, errors.New("pulp last successful pulp log not found")
	}

	// Parse the date from the memo
	var dateMemo pulpLogDateMemo
	err = json.Unmarshal(memo.Memo, &dateMemo)
	if err != nil {
		return now, err
	}

	date, err := time.Parse("2006-01-02", dateMemo.Date)
	if err != nil {
		return now, fmt.Errorf("failed to parse date from memo %s: %w", dateMemo.Date, err)
	}

	return date, nil
}

// SaveLastSuccessfulPulpLogDate saves the successful pulp log date to memo
func (mDao memoDaoImpl) SaveLastSuccessfulPulpLogDate(ctx context.Context, date time.Time) error {
	dateStr := date.Format("2006-01-02")
	dateMemo := pulpLogDateMemo{Date: dateStr}
	dateBytes, err := json.Marshal(dateMemo)
	if err != nil {
		return err
	}

	_, err = mDao.Write(ctx, config.MemoPulpLastSuccessfulPulpLogParse, dateBytes)
	return err
}
