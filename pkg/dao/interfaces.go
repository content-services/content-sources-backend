package dao

import (
	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/models"
)

type RepositoryDao interface {
	Create(newRepo api.RepositoryRequest) (api.RepositoryResponse, error)
	Update(orgID string, uuid string, repoParams api.RepositoryRequest) error
	Fetch(orgID string, uuid string) (api.RepositoryResponse, error)
	List(orgID string, paginationData api.PaginationData, filterData api.FilterData) (api.RepositoryCollectionResponse, int64, error)
	Delete(orgID string, uuid string) error
}

type RpmDao interface {
	Create(orgID string, accountID string, repo *models.Repository, newRpm *models.Rpm) error
	Fetch(OrgID string, AccountID string, rpmUUID string) (*api.RepositoryRpm, error)
	List(orgID string, accountID string, uuidRepo string, limit int, offset int) (api.RepositoryRpmCollectionResponse, int64, error)
}
