package dao

import (
	"context"
	"fmt"
	"time"

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

func (d metricsDaoImpl) PendingTasksCount(ctx context.Context) int64 {
	var output int64 = -1
	res := d.db.WithContext(ctx).
		Model(&models.TaskInfo{}).
		Where("status = ?", config.TaskStatusPending).Count(&output)
	if res.Error != nil {
		return output
	}
	return output
}

func (d metricsDaoImpl) PendingTasksAverageLatency(ctx context.Context) float64 {
	var output float64
	res := d.db.WithContext(ctx).
		Raw("SELECT  extract(epoch from (COALESCE(AVG(AGE(CURRENT_TIMESTAMP, queued_at)), INTERVAL '0 days'))) as latency FROM tasks WHERE status = ?", config.TaskStatusPending).
		Pluck("latency", &output)
	if res.Error != nil {
		return output
	}

	return output
}

func (d metricsDaoImpl) PendingTasksOldestTask(ctx context.Context) float64 {
	var task models.TaskInfo
	res := d.db.WithContext(ctx).
		Model(&models.TaskInfo{}).
		Where("status = ?", config.TaskStatusPending).Order("queued_at ASC").First(&task)
	if res.Error != nil {
		return 0
	}
	if task.Queued == nil {
		return 0
	}
	return time.Since(*task.Queued).Seconds()
}

func (d metricsDaoImpl) RHReposNoSuccessfulSnapshotTaskIn36Hours(ctx context.Context) int64 {
	var output int64 = -1
	date := time.Now().Add(-36 * time.Hour).Format(time.RFC3339)

	subQuery := d.db.WithContext(ctx).
		Model(&models.RepositoryConfiguration{}).
		Select("repository_configurations.uuid, bool_or(tasks.status ILIKE ? AND tasks.finished_at > ?) AS has_successful_tasks", fmt.Sprintf("%%%s%%", config.TaskStatusCompleted), date).
		Joins("LEFT OUTER JOIN tasks ON repository_configurations.repository_uuid = tasks.object_uuid").
		Where("repository_configurations.org_id = ?", config.RedHatOrg).
		Where("repository_configurations.snapshot IS TRUE").
		Where("tasks.type = ?", config.RepositorySnapshotTask).
		Group("repository_configurations.uuid")

	d.db.WithContext(ctx).
		Table("(?) AS sq", subQuery).
		Where("sq.has_successful_tasks IS FALSE").
		Count(&output)

	return output
}
