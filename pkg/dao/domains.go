package dao

import (
	"context"
	"fmt"

	"github.com/content-services/content-sources-backend/pkg/candlepin_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/pulp_client"
	"github.com/labstack/gommon/random"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type domainDaoImpl struct {
	db         *gorm.DB
	pulpClient pulp_client.PulpGlobalClient
	cpClient   candlepin_client.CandlepinClient
}

func GetDomainDao(db *gorm.DB,
	pulpClient pulp_client.PulpGlobalClient,
	candlepinClient candlepin_client.CandlepinClient) DomainDao {
	// Return DAO instance
	return domainDaoImpl{
		db:         db,
		pulpClient: pulpClient,
		cpClient:   candlepinClient,
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
	name := fmt.Sprintf("cs-%v", random.New().String(10, random.Hex))
	if orgId == config.RedHatOrg {
		name = config.RedHatDomainName
	}

	toCreate := models.Domain{
		DomainName: name,
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

func (dDao domainDaoImpl) RenameDomain(ctx context.Context, orgId string, newName string) error {
	domainName, err := dDao.Fetch(ctx, orgId)
	if err != nil {
		return fmt.Errorf("could not fetch domain name: %v", err)
	}

	templates := []models.Template{}
	res := dDao.db.WithContext(ctx).Where("org_id = ?", orgId).Find(&templates)
	if res.Error != nil {
		return fmt.Errorf("could not list templates for org: %v", res.Error)
	}
	pulpPath, err := dDao.pulpClient.GetContentPath(ctx)
	if err != nil {
		return fmt.Errorf("could not get pulp path: %v", err)
	}
	for _, template := range templates {
		prefix, err := config.EnvironmentPrefix(pulpPath, newName, template.UUID)
		if err != nil {
			return fmt.Errorf("could not get environment prefix: %v", err)
		}
		_, err = dDao.cpClient.UpdateEnvironmentPrefix(ctx, template.UUID, prefix)
		if err != nil {
			return fmt.Errorf("could not update environment prefix: %v", err)
		}
	}

	// Update it in pulp
	err = dDao.pulpClient.UpdateDomainName(ctx, domainName, newName)
	if err != nil {
		return fmt.Errorf("could not update pulp domain name: %v", err)
	}
	// Complete, so update the domain name in our db
	res = dDao.db.WithContext(ctx).Model(&models.Domain{}).Where("org_id = ?", orgId).Update("domain_name", newName)
	if res.Error != nil {
		return fmt.Errorf("could not update domain name in db: %v", res.Error)
	}
	return nil
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
