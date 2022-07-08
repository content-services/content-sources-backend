package dao

import (
	"github.com/content-services/content-sources-backend/pkg/api"
)

type RepositoryDao interface {
	Create(newRepo api.RepositoryRequest) (api.RepositoryResponse, error)
	Update(orgID string, uuid string, repoParams api.RepositoryRequest) error
	Fetch(orgID string, uuid string) (api.RepositoryResponse, error)
	List(orgID string, paginationData api.PaginationData, filterData api.FilterData) (api.RepositoryCollectionResponse, int64, error)
	Delete(orgID string, uuid string) error
}

type RpmDao interface {
	List(orgID string, uuidRepo string, limit int, offset int) (api.RepositoryRpmCollectionResponse, int64, error)
}
