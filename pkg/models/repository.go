package models

import (
	"time"

	"gorm.io/gorm"
)

// TODO Review the content for this table.
type Repository struct {
	Base
	// Repository URL
	URL string `json:"url" gorm:"not null"`
	// Last time the repo meta data was read
	LastReadTime *time.Time `gorm:"default:null"`
	// Last time the repo meta data failed to be read
	LastReadError *string `gorm:"default:null"`
	// ReferRepoConfig to Repository UUID
	ReferRepoConfig *string `gorm:"default:null"`
	// RepoConfig is the repository configuration
	RepoConfig *RepositoryConfiguration `gorm:"foreignKey:UUID;references:ReferRepoConfig"`
}

func (r *Repository) BeforeCreate(tx *gorm.DB) (err error) {
	if err := r.Base.BeforeCreate(tx); err != nil {
		return err
	}
	// TODO Add here any additional initialization
	return nil
}

func (r *Repository) DeepCopy() *Repository {
	return &Repository{
		Base: Base{
			UUID:      r.UUID,
			CreatedAt: r.CreatedAt,
			UpdatedAt: r.UpdatedAt,
		},
		URL:             r.URL,
		LastReadTime:    r.LastReadTime,
		LastReadError:   r.LastReadError,
		ReferRepoConfig: r.ReferRepoConfig,
		RepoConfig:      r.RepoConfig,
	}
}
