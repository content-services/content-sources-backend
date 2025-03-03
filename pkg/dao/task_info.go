package dao

import (
	"context"
	"errors"
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

	orgIDs := []string{config.RedHatOrg, orgID}
	result := t.db.WithContext(ctx).Table(taskInfo.TableName()+" AS t ").
		Select(JoinSelectQuery).
		Joins("LEFT JOIN repository_configurations rc on t.object_uuid = rc.repository_uuid AND t.object_type = ? AND rc.org_id in (?)", config.ObjectTypeRepository, orgIDs).
		Joins("LEFT JOIN templates on t.object_uuid = templates.uuid AND t.object_type = ? AND templates.org_id = ?", config.ObjectTypeTemplate, orgID).
		Joins("LEFT JOIN task_dependencies td on t.id = td.dependency_id").
		Where("t.id = ? AND t.org_id in (?) AND rc.deleted_at is NULL", UuidifyString(id), orgIDs).First(&taskInfo)

	if result.Error != nil {
		return taskInfoResponse, TasksDBToApiError(result.Error, &id)
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
		Joins("LEFT JOIN templates on t.object_uuid = templates.uuid AND t.object_type = ? AND templates.org_id = ?", config.ObjectTypeTemplate, orgID).
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
		return api.TaskInfoCollectionResponse{}, totalTasks, TasksDBToApiError(filteredDB.Error, nil)
	}

	// Most recently queued (created) first
	filteredDB.Order("queued_at DESC").Offset(pageData.Offset).Limit(pageData.Limit).Find(&tasks)

	if filteredDB.Error != nil {
		return api.TaskInfoCollectionResponse{}, totalTasks, TasksDBToApiError(filteredDB.Error, nil)
	}
	taskResponses := convertTaskInfoToResponses(tasks)
	return api.TaskInfoCollectionResponse{Data: taskResponses}, totalTasks, nil
}

func (t taskInfoDaoImpl) Cleanup(ctx context.Context) error {
	// Delete all completed or failed specified tasks that are older than 20 days
	// Delete all the canceled tasks that are older than 20 days (by queued time)
	// Delete all finished delete tasks that are older 10 days
	// Do not delete tasks if they are the last update-template-content task that updated a template
	q := "delete from tasks where " +
		"(type in (%v) and " +
		"NOT EXISTS (SELECT 1 FROM templates WHERE templates.last_update_task_uuid = tasks.id) AND" +
		"((status = 'completed' or status = 'failed') and finished_at < (current_date - interval '20' day)) OR" +
		"((status = 'canceled') and queued_at < (current_date - interval '20' day)))" +
		"OR" +
		"(type in (%v) and " +
		"status = 'completed' and  finished_at < (current_date - interval '10' day))"
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

func TasksDBToApiError(e error, uuid *string) *ce.DaoError {
	if e == nil {
		return nil
	}

	daoError := ce.DaoError{}
	if errors.Is(e, gorm.ErrRecordNotFound) {
		msg := "Task not found"
		if uuid != nil {
			msg = fmt.Sprintf("Task with UUID %s not found", *uuid)
		}
		daoError = ce.DaoError{
			Message:  msg,
			NotFound: true,
		}
	} else {
		daoError = ce.DaoError{
			Message:  e.Error(),
			NotFound: ce.HttpCodeForDaoError(e) == 404, // Check if isNotFoundError
		}
	}
	daoError.Wrap(e)
	return &daoError
}

func (t taskInfoDaoImpl) FetchActiveTasks(ctx context.Context, orgID string, objectUUID string, taskTypes ...string) ([]string, error) {
	taskInfo := make([]models.TaskInfo, 0)
	result := t.db.WithContext(ctx).
		Where("org_id = ?", orgID).
		Where("object_uuid = ?", UuidifyString(objectUUID)).
		Where("type in ?", taskTypes).
		Where("status = ? or status = ? or (status = ? and next_retry_time is not null)", config.TaskStatusPending, config.TaskStatusRunning, config.TaskStatusFailed).
		Find(&taskInfo)
	if result.Error != nil {
		return nil, result.Error
	}

	if len(taskInfo) == 0 {
		return nil, nil
	}

	uuids := make([]string, len(taskInfo))
	for i, task := range taskInfo {
		uuids[i] = task.Id.String()
	}

	return uuids, nil
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
