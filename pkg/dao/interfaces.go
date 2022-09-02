package dao

import (
	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/yummy/pkg/yum"
)

type RepositoryConfigDao interface {
	Create(newRepo api.RepositoryRequest) (api.RepositoryResponse, error)
	BulkCreate(newRepositories []api.RepositoryRequest) ([]api.RepositoryBulkCreateResponse, error)
	Update(orgID string, uuid string, repoParams api.RepositoryRequest) error
	Fetch(orgID string, uuid string) (api.RepositoryResponse, error)
	List(orgID string, paginationData api.PaginationData, filterData api.FilterData) (api.RepositoryCollectionResponse, int64, error)
	Delete(orgID string, uuid string) error
	SavePublicRepos(urls []string) error
	ValidateParameters(orgId string, params api.RepositoryValidationRequest, excludedUUIDS []string) (api.RepositoryValidationResponse, error)
}

type RpmDao interface {
	List(orgID string, uuidRepo string, limit int, offset int) (api.RepositoryRpmCollectionResponse, int64, error)
	Search(orgID string, request api.SearchRpmRequest, limit int) ([]api.SearchRpmResponse, error)
	InsertForRepository(repoUuid string, pkgs []yum.Package) (int64, error)
}

type RepositoryDao interface {
	FetchForUrl(url string) (error, Repository)
	List() (error, []Repository)
	Update(repo Repository) error
}

type ExternalResourceDao interface {
	ValidRepoMD(url string) (int, error)
}
