package dao

import (
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/models"
	"gorm.io/gorm"
)

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

func (d metricsDaoImpl) RepositoriesCount() int {
	// select COUNT(*) from repositories ;
	var output int64 = -1
	d.db.
		Model(&models.Repository{}).
		Count(&output)
	return int(output)
}

func (d metricsDaoImpl) RepositoryConfigsCount() int {
	// select COUNT(*) from repository_configurations ;
	var output int64 = -1
	d.db.
		Model(&models.RepositoryConfiguration{}).
		Count(&output)
	return int(output)
}

func (d metricsDaoImpl) PublicRepositoriesNotIntrospectedLas24HoursCount() int {
	// select COUNT(*) from repositories where last_introspection_update_time < NOW() - INTERVAL '24 hours' and public;
	var output int64 = -1
	d.db.
		Model(&models.Repository{}).
		// TODO Ask if we want to include the repositories which have not been introspected yet (Pending state)
		Where(d.db.Where("last_introspection_update_time < NOW() - INTERVAL '24 hours'").
			Or("last_introspection_update_time is NULL")).
		Where("public").
		Count(&output)
	return int(output)
}

func (d metricsDaoImpl) PublicRepositoriesFailedIntrospectionCount() int {
	// select COUNT(*)
	// from repositories
	// where status in ('Invalid','Unavailable')
	//   and public;
	var output int64 = -1
	d.db.
		Model(&models.Repository{}).
		Where("status in (?, ?)", config.StatusInvalid, config.StatusUnavailable).
		Where("public").
		Count(&output)
	return int(output)
}

func (d metricsDaoImpl) NonPublicRepositoriesNonIntrospectedLast24HoursCount() int {
	// select COUNT(*)
	//   from repositories
	//  where last_introspection_update_time < NOW() - INTERVAL '24 hours'
	//    and status in ('Invalid','Unavailable')
	//    and not public;
	var output int64 = -1
	d.db.
		Model(&models.Repository{}).
		Where("status in (?, ?)", config.StatusInvalid, config.StatusUnavailable).
		Where("last_introspection_update_time < NOW() - INTERVAL '24 hours'").
		Where("not public").
		Count(&output)
	return int(output)
}
