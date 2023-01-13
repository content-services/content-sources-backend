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
	// select COUNT(*)
	//   from repositories
	//  where public
	//    and status in ('Invalid','Unavailable')
	//    and (last_introspection_time < NOW() - INTERVAL '24 hours' or last_introspection_time is NULL);
	var output int64 = -1
	d.db.
		Model(&models.Repository{}).
		Where("public").
		Where("status in (?, ?)", config.StatusInvalid, config.StatusUnavailable).
		Where(d.db.Where("last_introspection_time is NULL").
			Or("last_introspection_time < NOW() - INTERVAL '24 hours'")).
		Count(&output)
	return int(output)
}

func (d metricsDaoImpl) PublicRepositoriesFailedIntrospectionCount() int {
	// select COUNT(*)
	// from repositories
	// where public
	//   and status in ('Invalid','Unavailable');
	var output int64 = -1
	d.db.
		Model(&models.Repository{}).
		Where("public").
		Where("status in (?, ?)", config.StatusInvalid, config.StatusUnavailable).
		Count(&output)
	return int(output)
}

func (d metricsDaoImpl) NonPublicRepositoriesNonIntrospectedLast24HoursCount() int {
	// select COUNT(*)
	//   from repositories
	//  where not public
	//    and status in ('Invalid','Unavailable')
	//    and (last_introspection_time < NOW() - INTERVAL '24 hours' or last_introspection_time is NULL);
	var output int64 = -1
	d.db.
		Model(&models.Repository{}).
		Where("not public").
		Where("status in (?, ?)", config.StatusInvalid, config.StatusUnavailable).
		Where(d.db.Where("last_introspection_time is NULL").
			Or("last_introspection_time < NOW() - INTERVAL '24 hours'")).
		Count(&output)
	return int(output)
}
