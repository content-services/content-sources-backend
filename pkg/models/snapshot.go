package models

import (
	"database/sql/driver"
	"encoding/json"
	"errors"

	"github.com/content-services/zest/release/v2026"
	"gorm.io/gorm"
)

const TableNameSnapshot = "snapshots"

type Snapshot struct {
	Base
	DeletedAt                   gorm.DeletedAt `json:"deleted_at"`
	VersionHref                 string         `json:"version_href" gorm:"not null"`
	PublicationHref             string         `json:"publication_href" gorm:"not null"`
	DistributionPath            string         `json:"distribution_path" gorm:"not null"`
	RepositoryPath              string         `json:"repository_path" gorm:"not null"` // Path to access the repository, includes domain
	DistributionHref            string         `json:"distribution_href" gorm:"not null"`
	RepositoryConfigurationUUID string         `json:"repository_configuration_uuid" gorm:"not null"`
	RepositoryConfiguration     RepositoryConfiguration
	ContentCounts               ContentCountsType `json:"content_counts" gorm:"not null,default:{}"`
	AddedCounts                 ContentCountsType `json:"added_counts" gorm:"not null,default:{}"`
	RemovedCounts               ContentCountsType `json:"removed_counts" gorm:"not null,default:{}"`
	DetectedOSVersion           string            `json:"detected_os_version" gorm:"not null"`
	Published                   bool              `json:"published" gorm:"default:false"`
}

type ContentCountsType map[string]int64

func (cc *ContentCountsType) Value() (driver.Value, error) {
	if *cc == nil {
		return "{}", nil
	}
	j, err := json.Marshal(cc)
	return j, err
}

func (cc *ContentCountsType) Scan(src interface{}) error {
	source, ok := src.([]byte)
	if !ok {
		return errors.New("type assertion .([]byte) failed")
	}

	var counts ContentCountsType
	err := json.Unmarshal(source, &counts)
	if err != nil {
		return err
	}

	*cc = counts
	return nil
}

func ContentSummaryToContentCounts(summary *zest.ContentSummaryResponse) (ContentCountsType, ContentCountsType, ContentCountsType) {
	presentCount := ContentCountsType{}
	addedCount := ContentCountsType{}
	removedCount := ContentCountsType{}
	if summary != nil {
		for contentType, item := range summary.Present {
			num, ok := item["count"].(float64)
			if ok {
				presentCount[contentType] = int64(num)
			}
		}
		for contentType, item := range summary.Added {
			num, ok := item["count"].(float64)
			if ok {
				addedCount[contentType] = int64(num)
			}
		}
		for contentType, item := range summary.Removed {
			num, ok := item["count"].(float64)
			if ok {
				removedCount[contentType] = int64(num)
			}
		}
	}
	return presentCount, addedCount, removedCount
}
