package models

import (
	"encoding/json"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const TableNameMemo = "memos"

// Memo model holds one-off pieces of data that can be read and updated
type Memo struct {
	UUID string          `json:"uuid" gorm:"primary_key"`
	Key  string          `json:"key" gorm:"uniqueIndex;not null"`
	Memo json.RawMessage `json:"memo" gorm:"type:jsonb;not null;default:'{}'"`
}

func (m *Memo) BeforeCreate(db *gorm.DB) (err error) {
	if m.UUID == "" {
		m.UUID = uuid.NewString()
	}
	return
}

func (m *Memo) TableName() string {
	return TableNameMemo
}
