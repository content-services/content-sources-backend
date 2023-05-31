package dao

import (
	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/yummy/pkg/yum"
	"gorm.io/gorm"
)

type DaoRegistry struct {
	RepositoryConfig RepositoryConfigDao
	Rpm              RpmDao
	Repository       RepositoryDao
	Metrics          MetricsDao
	Snapshot         SnapshotDao
	TaskInfo         TaskInfoDao
}

func GetDaoRegistry(db *gorm.DB) *DaoRegistry {
	reg := DaoRegistry{
		RepositoryConfig: repositoryConfigDaoImpl{
			db:      db,
			yumRepo: &yum.Repository{},
		},
		Rpm:        rpmDaoImpl{db: db},
		Repository: repositoryDaoImpl{db: db},
		Metrics:    metricsDaoImpl{db: db},
		Snapshot:   snapshotDaoImpl{db: db},
		TaskInfo:   taskInfoDaoImpl{db: db},
	}
	return &reg
}

//go:generate mockery --name RepositoryConfigDao --inpackage-suffix
type RepositoryConfigDao interface {
	Create(newRepo api.RepositoryRequest) (api.RepositoryResponse, error)
	BulkCreate(newRepositories []api.RepositoryRequest) ([]api.RepositoryResponse, []error)
	Update(orgID string, uuid string, repoParams api.RepositoryRequest) error
	Fetch(orgID string, uuid string) (api.RepositoryResponse, error)
	List(orgID string, paginationData api.PaginationData, filterData api.FilterData) (api.RepositoryCollectionResponse, int64, error)
	Delete(orgID string, uuid string) error
	SavePublicRepos(urls []string) error
	ValidateParameters(orgId string, params api.RepositoryValidationRequest, excludedUUIDS []string) (api.RepositoryValidationResponse, error)
	FetchByRepoUuid(orgID string, repoUuid string) (api.RepositoryResponse, error)
}

type RpmDao interface {
	List(orgID string, uuidRepo string, limit int, offset int, search string, sortBy string) (api.RepositoryRpmCollectionResponse, int64, error)
	Search(orgID string, request api.SearchRpmRequest) ([]api.SearchRpmResponse, error)
	InsertForRepository(repoUuid string, pkgs []yum.Package) (int64, error)
	OrphanCleanup() error
}

type RepositoryDao interface {
	FetchForUrl(url string) (Repository, error)
	List(ignoreFailed bool) ([]Repository, error)
	Update(repo RepositoryUpdate) error
	FetchRepositoryRPMCount(repoUUID string) (int, error)
	OrphanCleanup() error
}

type SnapshotDao interface {
	Create(snap *Snapshot) error
	List(repoConfigUuid string) ([]Snapshot, error)
}

type MetricsDao interface {
	RepositoriesCount() int
	RepositoryConfigsCount() int
	RepositoriesIntrospectionCount(hours int, public bool) IntrospectionCount
	PublicRepositoriesFailedIntrospectionCount() int
	OrganizationTotal() int64
}

type TaskInfoDao interface {
	Fetch(OrgID string, id string) (api.TaskInfoResponse, error)
	List(OrgID string, pageData api.PaginationData, statusFilter string) (api.TaskInfoCollectionResponse, int64, error)
}
