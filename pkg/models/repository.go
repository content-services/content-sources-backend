package models

import "time"

// TODO Review the content for this table.
type Repository struct {
	Base
	// Repository URL
	URL string `json:"url" gorm:"not null"`
	// Last time the repo meta data was read
	LastReadTime time.Time
	// Last time the repo meta data failed to be read
	LastReadError time.Time
	// ReferRepoConfig to Repository UUID
	ReferRepoConfig string `gorm:"not null"`
	// RepoConfig is the repository configuration
	RepoConfig *RepositoryConfiguration `gorm:"foreignKey:UUID;references:ReferRepoConfig"`
}
