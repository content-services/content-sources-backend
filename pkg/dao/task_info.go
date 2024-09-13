package dao

import (
	"context"
	"fmt"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

const JoinSelectQuery = ` t.id,
       t.type,
       t.payload,
       t.org_id,
       t.object_uuid,
	   t.object_type,
       t.token,
       t.queued_at,
       t.started_at,
       t.finished_at,
       t.error,
       t.status,
       t.request_id,
       rc.uuid as rc_uuid,
       rc.name as rc_name,
       templates.uuid as template_uuid,
	   templates.name as template_name,
       ARRAY (SELECT td.dependency_id FROM task_dependencies td WHERE td.task_id = t.id) as t_dependencies,
       ARRAY (SELECT td.task_id FROM task_dependencies td WHERE td.dependency_id = t.id) as t_dependents
`

type taskInfoDaoImpl struct {
	db *gorm.DB
}

func GetTaskInfoDao(db *gorm.DB) TaskInfoDao {
	return taskInfoDaoImpl{
		db: db,
	}
}

func (t taskInfoDaoImpl) Fetch(ctx context.Context, orgID string, id string) (api.TaskInfoResponse, error) {
	taskInfo := models.TaskInfoRepositoryConfiguration{}
	taskInfoResponse := api.TaskInfoResponse{}

	result := t.db.WithContext(ctx).Table(taskInfo.TableName()+" AS t ").
		Select(JoinSelectQuery).
		Joins("LEFT JOIN repository_configurations rc on t.object_uuid = rc.repository_uuid AND rc.org_id = ? and t.object_type = ?", orgID, config.ObjectTypeRepository).
		Joins("LEFT JOIN templates on t.object_uuid = templates.uuid AND t.object_type = ? AND templates.org_id in (?)", config.ObjectTypeTemplate, []string{config.RedHatOrg, orgID}).
		Joins("LEFT JOIN task_dependencies td on t.id = td.dependency_id").
		Where("t.id = ? AND t.org_id in (?) AND rc.deleted_at is NULL", UuidifyString(id), []string{config.RedHatOrg, orgID}).First(&taskInfo)

	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return taskInfoResponse, &ce.DaoError{NotFound: true, Message: "Could not find task with UUID " + id}
		} else {
			return taskInfoResponse, DBErrorToApi(result.Error)
		}
	}
	taskInfoModelToApiFields(&taskInfo, &taskInfoResponse)
	return taskInfoResponse, nil
}

func (t taskInfoDaoImpl) List(
	ctx context.Context,
	orgID string,
	pageData api.PaginationData,
	filterData api.TaskInfoFilterData,
) (api.TaskInfoCollectionResponse, int64, error) {
	var totalTasks int64

	var taskInfo models.TaskInfo
	tasks := make([]models.TaskInfoRepositoryConfiguration, 0)

	var orgsForQuery []string
	if filterData.ExcludeRedHatOrg {
		orgsForQuery = []string{orgID}
	} else {
		orgsForQuery = []string{config.RedHatOrg, orgID}
	}

	filteredDB := t.db.WithContext(ctx).Table(taskInfo.TableName()+" AS t ").
		Select(JoinSelectQuery).
		Joins("LEFT JOIN repository_configurations rc on t.object_uuid = rc.repository_uuid AND t.object_type = ? AND rc.org_id in (?)", config.ObjectTypeRepository, []string{config.RedHatOrg, orgID}).
		Joins("LEFT JOIN templates on t.object_uuid = templates.uuid AND t.object_type = ? AND templates.org_id in (?)", config.ObjectTypeTemplate, []string{config.RedHatOrg, orgID}).
		Where("t.org_id in (?) AND rc.deleted_at is NULL", orgsForQuery)

	if filterData.Status != "" {
		filteredDB = filteredDB.Where("t.status = ?", filterData.Status)
	}

	if filterData.Typename != "" {
		filteredDB = filteredDB.Where("t.type = ?", filterData.Typename)
	}

	if filterData.RepoConfigUUID != "" {
		query := "rc.uuid = ?"
		args := []interface{}{UuidifyString(filterData.RepoConfigUUID)}
		if filterData.TemplateUUID != "" {
			query = fmt.Sprintf("%s OR templates.uuid = ?", query)
			args = append(args, UuidifyString(filterData.TemplateUUID))
		}
		filteredDB = filteredDB.Where(query, args...)
	} else if filterData.TemplateUUID != "" {
		filteredDB = filteredDB.Where("templates.uuid = ?", UuidifyString(filterData.TemplateUUID))
	}

	// First get count
	filteredDB.Model(&tasks).Count(&totalTasks)

	if filteredDB.Error != nil {
		return api.TaskInfoCollectionResponse{}, totalTasks, DBErrorToApi(filteredDB.Error)
	}

	// Most recently queued (created) first
	filteredDB.Order("queued_at DESC").Offset(pageData.Offset).Limit(pageData.Limit).Find(&tasks)

	if filteredDB.Error != nil {
		return api.TaskInfoCollectionResponse{}, totalTasks, DBErrorToApi(filteredDB.Error)
	}
	taskResponses := convertTaskInfoToResponses(tasks)
	return api.TaskInfoCollectionResponse{Data: taskResponses}, totalTasks, nil
}

func (t taskInfoDaoImpl) Cleanup(ctx context.Context) error {
	// Delete all completed or failed specified tasks that are older than 10 days
	// Delete all finished delete tasks that are older 10 days
	q := "delete from tasks where " +
		"(type in (%v) and (status = 'completed' or status = 'failed') and finished_at < (current_date - interval '20' day)) OR" +
		"(type in (%v) and status = 'completed' and  finished_at < (current_date - interval '10' day))"
	q = fmt.Sprintf(q, stringSliceToQueryList(config.TasksToCleanup), stringSliceToQueryList(config.TasksToCleanupIfCompleted))
	result := t.db.WithContext(ctx).Exec(q)
	if result.Error != nil {
		return result.Error
	}
	log.Logger.Debug().Msgf("Cleaned up %v old tasks", result.RowsAffected)

	// Delete all snapshot tasks that no longer have repo configs (User deleted their repository)
	orphanQ := "DELETE FROM tasks WHERE id IN ( " +
		"SELECT t.id FROM tasks AS t " +
		"LEFT JOIN repository_configurations AS rc ON t.org_id = rc.org_id and t.object_uuid = rc.repository_uuid AND t.object_type = ? " +
		"WHERE t.object_uuid is NOT NULL AND rc.repository_uuid is null AND t.type = ?)"

	result = t.db.WithContext(ctx).Exec(orphanQ, config.ObjectTypeRepository, config.RepositorySnapshotTask)
	if result.Error != nil {
		return result.Error
	}
	log.Logger.Debug().Msgf("Cleaned up %v orphan snapshot tasks", result.RowsAffected)
	return nil
}

func (t taskInfoDaoImpl) IsTaskInProgressOrPending(ctx context.Context, orgID, objectUUID, taskType string) (bool, string, error) {
	taskInfo := models.TaskInfo{}
	result := t.db.WithContext(ctx).Where("org_id = ? and object_uuid = ? and (status = ? or status = ?) and type = ?",
		orgID, objectUUID, config.TaskStatusRunning, config.TaskStatusPending, taskType).First(&taskInfo)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return false, "", nil
		} else {
			return false, "", result.Error
		}
	}
	if taskInfo.Status == config.TaskStatusRunning || taskInfo.Status == config.TaskStatusPending {
		return true, taskInfo.Id.String(), nil
	}
	return false, taskInfo.Id.String(), nil
}

func taskInfoModelToApiFields(taskInfo *models.TaskInfoRepositoryConfiguration, apiTaskInfo *api.TaskInfoResponse) {
	apiTaskInfo.UUID = taskInfo.Id.String()
	apiTaskInfo.OrgId = taskInfo.OrgId
	apiTaskInfo.Status = taskInfo.Status
	apiTaskInfo.Typename = taskInfo.Typename
	apiTaskInfo.Dependencies = taskInfo.Dependencies
	apiTaskInfo.Dependents = taskInfo.Dependents

	if taskInfo.ObjectType != nil {
		switch *taskInfo.ObjectType {
		case config.ObjectTypeTemplate:
			apiTaskInfo.ObjectType = *taskInfo.ObjectType
			apiTaskInfo.ObjectUUID = taskInfo.TemplateUUID
			apiTaskInfo.ObjectName = taskInfo.TemplateName
		case config.ObjectTypeRepository:
			apiTaskInfo.ObjectType = *taskInfo.ObjectType
			apiTaskInfo.ObjectUUID = taskInfo.RepositoryConfigUUID
			apiTaskInfo.ObjectName = taskInfo.RepositoryConfigName
		}
	}

	if taskInfo.Error != nil {
		apiTaskInfo.Error = *taskInfo.Error
	}

	if taskInfo.Queued != nil {
		apiTaskInfo.CreatedAt = taskInfo.Queued.Format(time.RFC3339)
	}

	if taskInfo.Finished != nil {
		apiTaskInfo.EndedAt = taskInfo.Finished.Format(time.RFC3339)
	}
}

func convertTaskInfoToResponses(taskInfo []models.TaskInfoRepositoryConfiguration) []api.TaskInfoResponse {
	tasks := make([]api.TaskInfoResponse, len(taskInfo))
	for i := range taskInfo {
		taskInfoModelToApiFields(&taskInfo[i], &tasks[i])
	}
	return tasks
}

func stringSliceToQueryList(s []string) string {
	var query string
	for i, v := range s {
		query += fmt.Sprintf("'%v'", v)
		if i < len(s)-1 {
			query += ","
		}
	}
	return query
}
