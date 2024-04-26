package dao

import (
	"context"

	"github.com/content-services/content-sources-backend/pkg/models"
	uuid2 "github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type domainDaoImpl struct {
	db *gorm.DB
}

func GetDomainDao(db *gorm.DB) DomainDao {
	// Return DAO instance
	return domainDaoImpl{
		db: db,
	}
}

func (dDao domainDaoImpl) FetchOrCreateDomain(ctx context.Context, orgId string) (string, error) {
	dName, err := dDao.Fetch(ctx, orgId)
	if err != nil {
		return "", err
	} else if dName != "" {
		return dName, nil
	}
	return dDao.Create(ctx, orgId)
}

func (dDao domainDaoImpl) Create(ctx context.Context, orgId string) (string, error) {
	toCreate := models.Domain{
		DomainName: uuid2.NewString()[0:8],
		OrgId:      orgId,
	}
	result := dDao.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "org_id"}},
		DoNothing: true,
	}).Create(&toCreate)

	if result.Error != nil {
		return "", result.Error
	} else {
		return dDao.Fetch(ctx, orgId)
	}
}

func (dDao domainDaoImpl) Fetch(ctx context.Context, orgId string) (string, error) {
	var found []models.Domain
	result := dDao.db.WithContext(ctx).Where("org_id = ?", orgId).Find(&found)
	if result.Error != nil {
		return "", result.Error
	}
	if len(found) != 1 {
		return "", nil
	}
	return found[0].DomainName, nil
}
