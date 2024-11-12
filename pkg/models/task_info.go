package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"
)

// Shared by DAO and queue packages
// GORM only used in DAO to read from table
type TaskInfo struct {
	Id              uuid.UUID       `gorm:"primary_key;column:id"`
	Typename        string          `gorm:"column:type"` // "introspect" or "snapshot"
	Payload         json.RawMessage `gorm:"type:jsonb"`
	OrgId           string
	AccountId       string
	ObjectUUID      uuid.UUID
	ObjectType      *string
	Dependencies    pq.StringArray `gorm:"->;column:t_dependencies;type:text[]"`
	Dependents      pq.StringArray `gorm:"->;column:t_dependents;type:text[]"`
	Token           uuid.UUID
	Queued          *time.Time `gorm:"column:queued_at"`
	Started         *time.Time `gorm:"column:started_at"`
	Finished        *time.Time `gorm:"column:finished_at"`
	Error           *string
	Status          string
	RequestID       string
	Retries         int
	NextRetryTime   *time.Time
	Priority        int
	CancelAttempted bool
}

type TaskInfoRepositoryConfiguration struct {
	*TaskInfo
	RepositoryConfigUUID string `gorm:"column:rc_uuid"`
	RepositoryConfigName string `gorm:"column:rc_name"`
	TemplateUUID         string `gorm:"column:template_uuid"`
	TemplateName         string `gorm:"column:template_name"`
}

func (t *TaskInfo) AfterFind(tx *gorm.DB) error {
	if t.Queued != nil {
		*t.Queued = t.Queued.UTC()
	}
	if t.Started != nil {
		*t.Started = t.Started.UTC()
	}
	if t.Finished != nil {
		*t.Finished = t.Finished.UTC()
	}
	if t.NextRetryTime != nil {
		*t.NextRetryTime = t.NextRetryTime.UTC()
	}
	return nil
}

func (t *TaskInfo) AfterSave(tx *gorm.DB) error {
	return t.AfterFind(tx)
}

func (*TaskInfo) TableName() string {
	return "tasks"
}
