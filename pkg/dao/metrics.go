package dao

import (
	"context"
	"fmt"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

type IntrospectionCount struct {
	Introspected int64
	Missed       int64
}

type metricsDaoImpl struct {
	db *gorm.DB
}

func GetMetricsDao(db *gorm.DB) MetricsDao {
	if db == nil {
		return nil
	}
	return metricsDaoImpl{
		db: db,
	}
}

func (d metricsDaoImpl) RepositoriesCount(ctx context.Context) int {
	// select COUNT(*) from repositories ;
	var output int64 = -1
	d.db.WithContext(ctx).
		Model(&models.Repository{}).
		Count(&output)
	return int(output)
}

func (d metricsDaoImpl) RepositoryConfigsCount(ctx context.Context) int {
	// select COUNT(*) from repository_configurations ;
	var output int64 = -1
	d.db.WithContext(ctx).
		Model(&models.RepositoryConfiguration{}).
		Count(&output)
	return int(output)
}

func (d metricsDaoImpl) OrganizationTotal(ctx context.Context) int64 {
	var output int64 = -1
	tx := d.db.WithContext(ctx).
		Model(&models.RepositoryConfiguration{}).Group("org_id").
		Count(&output)
	if tx.Error != nil {
		log.Error().Err(tx.Error).Msg("Cannot calculate OrganizationTotal")
	}
	return output
}

func (d metricsDaoImpl) RepositoriesIntrospectionCount(ctx context.Context, hours int, public bool) IntrospectionCount {
	// select COUNT(*)
	//   from repositories
	//  where public
	//	  and failed_introspections_count <= FailedIntrospectionsLimit
	//    and (last_introspection_time < NOW() - INTERVAL '24 hours' or last_introspection_time is NULL);
	output := IntrospectionCount{
		Introspected: -1,
		Missed:       -1,
	}
	interval := fmt.Sprintf("%v hours", hours)
	publicClause := "not public"
	if public {
		publicClause = "public"
	}

	tx := d.db.WithContext(ctx).Model(&models.Repository{}).
		Where(publicClause).Where(d.db.Where("last_introspection_time is NULL and last_introspection_status != ?", config.StatusPending).
		Where("failed_introspections_count <= ?", config.FailedIntrospectionsLimit).
		Or("last_introspection_time < NOW() - cast(? as INTERVAL)", interval)).
		Count(&output.Missed)
	if tx.Error != nil {
		log.Logger.Err(tx.Error).Msg("error")
	}

	tx = d.db.WithContext(ctx).Model(&models.Repository{}).
		Where(publicClause).Where("last_introspection_time >= NOW() - cast(? as INTERVAL)", interval).
		Count(&output.Introspected)
	if tx.Error != nil {
		log.Logger.Err(tx.Error).Msg("Error")
	}
	return output
}

func (d metricsDaoImpl) PublicRepositoriesFailedIntrospectionCount(ctx context.Context) int {
	// select COUNT(*)
	// from repositories
	// where public
	//   and last_introspection_status in ('Invalid','Unavailable');
	var output int64 = -1
	d.db.WithContext(ctx).
		Model(&models.Repository{}).
		Where("public").
		Where("last_introspection_status in (?, ?)", config.StatusInvalid, config.StatusUnavailable).
		Count(&output)
	return int(output)
}
