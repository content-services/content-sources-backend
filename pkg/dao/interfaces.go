package dao

import (
	"github.com/content-services/content-sources-backend/pkg/api"
)

type RepositoryDao interface {
	Create(r api.Repository) error
}
