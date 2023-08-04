package models

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
)

type Snapshot struct {
	Base
	VersionHref      string `json:"version_href" gorm:"not null"`
	PublicationHref  string `json:"publication_href" gorm:"not null"`
	DistributionPath string `json:"distribution_path" gorm:"not null"`
	RepositoryPath   string `json:"repository_path" gorm:"not null"` // Path to access the repository, includes domain
	DistributionHref string `json:"distribution_href" gorm:"not null"`
	OrgId            string `json:"org_id" gorm:"not null"`
	RepositoryUUID   string `json:"repository_uuid" gorm:"not null"`
	Repository       Repository
	ContentCounts    ContentCounts `json:"content_counts" gorm:"not null,default:{}"`
}

type ContentCounts map[string]int64

func (cc *ContentCounts) Value() (driver.Value, error) {
	if *cc == nil {
		return "{}", nil
	}
	j, err := json.Marshal(cc)
	return j, err
}

func (cc *ContentCounts) Scan(src interface{}) error {
	source, ok := src.([]byte)
	if !ok {
		return errors.New("Type assertion .([]byte) failed.")
	}

	var counts ContentCounts
	err := json.Unmarshal(source, &counts)
	if err != nil {
		return err
	}

	*cc = counts
	return nil
}
