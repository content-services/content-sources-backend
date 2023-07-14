package models

import (
	"encoding/json"
	"time"

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
