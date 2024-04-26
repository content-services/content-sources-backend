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
       t.repository_uuid,
       t.token,
       t.queued_at,
       t.started_at,
       t.finished_at,
       t.error,
       t.status,
       t.request_id,
       rc.uuid as rc_uuid,
       rc.name as rc_name
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
		Joins("LEFT JOIN repository_configurations rc on t.repository_uuid = rc.repository_uuid AND rc.org_id = ?", orgID).
		Where("t.id = ? AND t.org_id in (?)", UuidifyString(id), []string{config.RedHatOrg, orgID}).First(&taskInfo)

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

	filteredDB := t.db.WithContext(ctx).Table(taskInfo.TableName()+" AS t ").
		Select(JoinSelectQuery).
		Joins("LEFT JOIN repository_configurations rc on t.repository_uuid = rc.repository_uuid  AND rc.org_id in (?)", []string{config.RedHatOrg, orgID}).
		Where("t.org_id in (?)", []string{config.RedHatOrg, orgID})

	if filterData.Status != "" {
		filteredDB = filteredDB.Where("t.status = ?", filterData.Status)
	}

	if filterData.Typename != "" {
		filteredDB = filteredDB.Where("t.type = ?", filterData.Typename)
	}

	if filterData.RepoConfigUUID != "" {
		filteredDB = filteredDB.Where("rc.uuid = ?", filterData.RepoConfigUUID)
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
	// Delete all completed or failed introspection tasks that are older than 10 days
	// Delete all finished Repo delete tasks that are older 10 days
	q := "delete from tasks where " +
		"(type = '%v' and (status = 'completed' or status = 'failed') and finished_at < (current_date - interval '10' day)) OR" +
		"(type = '%v' and status = 'completed' and  finished_at < (current_date - interval '10' day))"
	q = fmt.Sprintf(q, config.IntrospectTask, config.DeleteRepositorySnapshotsTask)
	result := t.db.WithContext(ctx).Exec(q)
	if result.Error != nil {
		return result.Error
	}
	log.Logger.Debug().Msgf("Cleaned up %v old tasks", result.RowsAffected)

	// Delete all snapshot tasks that no longer have repo configs (User deleted their repository)
	orphanQ := "DELETE FROM tasks WHERE id IN ( " +
		"SELECT t.id FROM tasks AS t " +
		"LEFT JOIN repository_configurations AS rc ON t.org_id = rc.org_id and t.repository_uuid = rc.repository_uuid " +
		"WHERE t.repository_uuid is NOT NULL AND rc.repository_uuid is null AND t.type = '%v')"
	orphanQ = fmt.Sprintf(orphanQ, config.RepositorySnapshotTask)

	result = t.db.WithContext(ctx).Exec(orphanQ)
	if result.Error != nil {
		return result.Error
	}
	log.Logger.Debug().Msgf("Cleaned up %v orphan snapshot tasks", result.RowsAffected)
	return nil
}

func (t taskInfoDaoImpl) IsSnapshotInProgress(ctx context.Context, orgID, repoUUID string) (bool, error) {
	taskInfo := models.TaskInfo{}
	result := t.db.WithContext(ctx).Where("org_id = ? and repository_uuid = ? and status = ? and type = ?",
		orgID, repoUUID, config.TaskStatusRunning, config.RepositorySnapshotTask).First(&taskInfo)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return false, nil
		} else {
			return false, result.Error
		}
	}
	if taskInfo.Status == config.TaskStatusRunning {
		return true, nil
	}
	return false, nil
}

func taskInfoModelToApiFields(taskInfo *models.TaskInfoRepositoryConfiguration, apiTaskInfo *api.TaskInfoResponse) {
	apiTaskInfo.UUID = taskInfo.Id.String()
	apiTaskInfo.OrgId = taskInfo.OrgId
	apiTaskInfo.Status = taskInfo.Status
	apiTaskInfo.Typename = taskInfo.Typename
	apiTaskInfo.RepoConfigUUID = taskInfo.RepositoryConfigUUID
	apiTaskInfo.RepoConfigName = taskInfo.RepositoryConfigName

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
