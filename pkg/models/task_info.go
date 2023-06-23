package models

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/tasks/payloads"
	"github.com/google/uuid"
)

// Shared by DAO and queue packages
// GORM only used in DAO to read from table
type TaskInfo struct {
	Id             uuid.UUID       `gorm:"primary_key;column:id"`
	Typename       string          `gorm:"column:type"` // "introspect" or "snapshot"
	Payload        json.RawMessage `gorm:"type:jsonb"`
	OrgId          string
	RepositoryUUID uuid.UUID
	Dependencies   []uuid.UUID `gorm:"-"`
	Token          uuid.UUID
	Queued         *time.Time `gorm:"column:queued_at"`
	Started        *time.Time `gorm:"column:started_at"`
	Finished       *time.Time `gorm:"column:finished_at"`
	Error          *string
	Status         string
}

func (*TaskInfo) TableName() string {
	return "tasks"
}

func (ti *TaskInfo) GetPulpData(pulpClient pulp_client.PulpClient) (api.PulpResponse, error) {
	if ti.Typename == payloads.Snapshot {
		var payload payloads.SnapshotPayload
		response := api.PulpResponse{}

		if err := json.Unmarshal(ti.Payload, &payload); err != nil {
			return api.PulpResponse{}, errors.New("invalid snapshot payload")
		}

		if payload.SyncTaskHref != nil {
			sync, syncErr := pulpClient.GetTask(*payload.SyncTaskHref)
			if syncErr != nil {
				return api.PulpResponse{}, syncErr
			}
			response.Sync = &api.PulpTaskResponse{}
			api.ZestTaskResponseToApi(&sync, response.Sync)
		}

		if payload.DistributionTaskHref != nil {
			distribution, distributionErr := pulpClient.GetTask(*payload.DistributionTaskHref)
			if distributionErr != nil {
				return api.PulpResponse{}, distributionErr
			}
			response.Distribution = &api.PulpTaskResponse{}
			api.ZestTaskResponseToApi(&distribution, response.Distribution)
		}

		if payload.PublicationTaskHref != nil {
			publication, publicationErr := pulpClient.GetTask(*payload.PublicationTaskHref)
			if publicationErr != nil {
				return api.PulpResponse{}, publicationErr
			}
			response.Publication = &api.PulpTaskResponse{}
			api.ZestTaskResponseToApi(&publication, response.Publication)
		}

		return response, nil
	}
	return api.PulpResponse{}, errors.New("incorrect task type")
}
