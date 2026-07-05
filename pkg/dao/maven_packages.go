package dao

import (
	"context"
	"fmt"

	"github.com/content-services/content-sources-backend/pkg/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type mavenPackagesDaoImpl struct {
	db *gorm.DB
}

func GetMavenPackagesDao(db *gorm.DB) MavenPackagesDao {
	return mavenPackagesDaoImpl{db: db}
}

func (d mavenPackagesDaoImpl) Create(ctx context.Context, mavenPackage *models.MavenPackage) error {
	if mavenPackage == nil {
		return fmt.Errorf("maven package cannot be nil")
	}
	if mavenPackage.Name == "" {
		return fmt.Errorf("name cannot be empty")
	}

	result := d.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "name"}},
		DoNothing: true,
	}).Create(mavenPackage)
	if result.Error != nil {
		return fmt.Errorf("failed to create maven package: %w", result.Error)
	}

	return nil
}

func (d mavenPackagesDaoImpl) Fetch(ctx context.Context, name string) (*models.MavenPackage, error) {
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}

	var mavenPackage models.MavenPackage
	result := d.db.WithContext(ctx).Where("name = ?", name).First(&mavenPackage)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to fetch maven package: %w", result.Error)
	}

	return &mavenPackage, nil
}
