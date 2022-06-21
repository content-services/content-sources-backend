package dao

import (
	"github.com/content-services/content-sources-backend/pkg/api"
)

type RepositoryDao interface {
	Create(newRepo api.RepositoryRequest) error
	Update(orgID string, uuid string, repoParams api.RepositoryRequest) error
	Fetch(orgID string, uuid string) (api.RepositoryResponse, error)
	List(orgID string, paginationData api.PaginationData, filterData api.FilterData) (api.RepositoryCollectionResponse, int64, error)
	Delete(orgID string, uuid string) error
}
