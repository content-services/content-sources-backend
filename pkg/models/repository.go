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
	// Refer to Repository UUID
	Refer2RepoConfig string `gorm:"not null"`
	// Repo is the repository configuration
	RepoConfig *RepositoryConfiguration `gorm:"foreignKey:UUID;references:Refer2RepoConfig"`
}
