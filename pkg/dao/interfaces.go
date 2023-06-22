package dao

import (
	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/pulp_client"
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
	AdminTask        AdminTaskDao
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
		AdminTask:  adminTaskInfoDaoImpl{db: db, pulpClient: pulp_client.GetPulpClient()},
	}
	return &reg
}

//go:generate mockery --name RepositoryConfigDao --filename repository_configs_mock.go --inpackage
type RepositoryConfigDao interface {
	Create(newRepo api.RepositoryRequest) (api.RepositoryResponse, error)
	BulkCreate(newRepositories []api.RepositoryRequest) ([]api.RepositoryResponse, []error)
	Update(orgID, uuid string, repoParams api.RepositoryRequest) (bool, error)
	Fetch(orgID string, uuid string) (api.RepositoryResponse, error)
	List(orgID string, paginationData api.PaginationData, filterData api.FilterData) (api.RepositoryCollectionResponse, int64, error)
	Delete(orgID string, uuid string) error
	BulkDelete(orgID string, uuids []string) []error
	SavePublicRepos(urls []string) error
	ValidateParameters(orgId string, params api.RepositoryValidationRequest, excludedUUIDS []string) (api.RepositoryValidationResponse, error)
	FetchByRepoUuid(orgID string, repoUuid string) (api.RepositoryResponse, error)
	InternalOnly_FetchRepoConfigsForRepoUUID(uuid string) []api.RepositoryResponse
}

//go:generate mockery --name RpmDao --filename rpms_mock.go --inpackage
type RpmDao interface {
	List(orgID string, uuidRepo string, limit int, offset int, search string, sortBy string) (api.RepositoryRpmCollectionResponse, int64, error)
	Search(orgID string, request api.SearchRpmRequest) ([]api.SearchRpmResponse, error)
	InsertForRepository(repoUuid string, pkgs []yum.Package) (int64, error)
	OrphanCleanup() error
}

//go:generate mockery --name RepositoryDao --filename repositories_mock.go --inpackage
type RepositoryDao interface {
	FetchForUrl(url string) (Repository, error)
	List(ignoreFailed bool) ([]Repository, error)
	ListPublic(paginationData api.PaginationData, _ api.FilterData) (api.PublicRepositoryCollectionResponse, int64, error)
	Update(repo RepositoryUpdate) error
	FetchRepositoryRPMCount(repoUUID string) (int, error)
	OrphanCleanup() error
}

//go:generate mockery --name SnapshotDao --filename snapshots_mock.go --inpackage
type SnapshotDao interface {
	Create(snap *models.Snapshot) error
	List(repoConfigUuid string, paginationData api.PaginationData, filterData api.FilterData) (api.SnapshotCollectionResponse, int64, error)
	FetchForRepoUUID(orgID string, repoUUID string) ([]models.Snapshot, error)
	Delete(snapUUID string) error
}

//go:generate mockery --name MetricsDao --filename metrics_mock.go --inpackage
type MetricsDao interface {
	RepositoriesCount() int
	RepositoryConfigsCount() int
	RepositoriesIntrospectionCount(hours int, public bool) IntrospectionCount
	PublicRepositoriesFailedIntrospectionCount() int
	OrganizationTotal() int64
}

//go:generate mockery --name TaskInfoDao --filename task_info_mock.go --inpackage
type TaskInfoDao interface {
	Fetch(OrgID string, id string) (api.TaskInfoResponse, error)
	List(OrgID string, pageData api.PaginationData, statusFilter string) (api.TaskInfoCollectionResponse, int64, error)
	IsSnapshotInProgress(orgID, repoUUID string) (bool, error)
}

type AdminTaskDao interface {
	Fetch(id string) (api.AdminTaskInfoResponse, error)
	List(pageData api.PaginationData, filterData api.AdminTaskFilterData) (api.AdminTaskInfoCollectionResponse, int64, error)
}
