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
	// TODO Remove debug
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
	// where (status = 'Invalid' or status = 'Unavailable')
	//   and public;
	// TODO Remove debug
	var output int64 = -1
	d.db.Debug().
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
	//    and not public;
	// TODO Remove debug
	var output int64 = -1
	d.db.Debug().
		Model(&models.Repository{}).
		Where("status in (?, ?)", config.StatusInvalid, config.StatusUnavailable).
		Where("last_introspection_update_time < NOW() - INTERVAL '24 hours'").
		Where("not public").
		Count(&output)
	return int(output)
}

// func (d metricsDaoImpl) Top50Repositories() []map[string]interface{} {
// 	// select COUNT(repository_configurations.*) as repo_count, repositories.url
// 	//   from repository_configurations
// 	//   inner join repositories on repository_configurations.repository_uuid = repositories.uuid
// 	//   group by repositories.uuid
// 	//   order by repo_count desc
// 	//   limit 50;

// 	// TODO Remove debug
// 	rows, err := d.db.Debug().
// 		Model(&models.Repository{}).
// 		Joins("inner join repository_configurations ON repository_configurations.repository_uuid = repositories.uuid").
// 		// Where("last_introspection_update_time < NOW() - INTERVAL '24 hours'").
// 		// Where("not public").
// 		Select("COUNT(repository_configurations.*) as repo_count, repositories.url as url").
// 		Group("repositories.uuid").
// 		Order("repo_count desc").
// 		// Count(&count).
// 		Limit(50).
// 		Rows()
// 	if err != nil {
// 		log.Error().Err(err).Msg("Top50Repositories")
// 		return []map[string]interface{}{}
// 	}
// 	defer rows.Close()

// 	type Row struct {
// 		Count int64
// 		Url   string
// 	}
// 	output := []map[string]interface{}{}
// 	for rows.Next() {
// 		var row Row
// 		if err := d.db.ScanRows(rows, &row); err != nil {
// 			return []map[string]interface{}{}
// 		}
// 		item := map[string]interface{}{}
// 		item["repo_count"] = row.Count
// 		item["url"] = row.Url
// 		output = append(output, item)
// 	}

// 	return output
// }
