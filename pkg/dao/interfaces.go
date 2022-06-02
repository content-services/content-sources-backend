package dao

import (
	"github.com/content-services/content-sources-backend/pkg/api"
)

type RepositoryDao interface {
	Create(newRepo api.RepositoryRequest) error
	Fetch(orgId string, uuid string) api.RepositoryResponse
	Update(orgId string, uuid string, repoParams api.RepositoryRequest) error
}
