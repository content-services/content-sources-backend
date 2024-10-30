package dao

import (
	"context"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/pulp_client"
	"github.com/content-services/tang/pkg/tangy"
	"github.com/content-services/yummy/pkg/yum"
	"github.com/rs/zerolog/log"
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
	Domain           DomainDao
	PackageGroup     PackageGroupDao
	Environment      EnvironmentDao
	Template         TemplateDao
}

func GetDaoRegistry(db *gorm.DB) *DaoRegistry {
	reg := DaoRegistry{
		RepositoryConfig: &repositoryConfigDaoImpl{
			db:         db,
			yumRepo:    &yum.Repository{},
			pulpClient: pulp_client.GetPulpClientWithDomain(""),
		},
		Rpm: &rpmDaoImpl{
			db: db,
		},
		Repository: repositoryDaoImpl{db: db},
		Metrics:    metricsDaoImpl{db: db},
		Snapshot: &snapshotDaoImpl{
			db:         db,
			pulpClient: pulp_client.GetPulpClientWithDomain(""),
		},
		TaskInfo:     taskInfoDaoImpl{db: db},
		AdminTask:    adminTaskInfoDaoImpl{db: db, pulpClient: pulp_client.GetGlobalPulpClient()},
		Domain:       domainDaoImpl{db: db},
		PackageGroup: packageGroupDaoImpl{db: db},
		Environment:  environmentDaoImpl{db: db},
		Template:     templateDaoImpl{db: db},
	}
	return &reg
}

// SetupGormTableOrFail this is necessary to enable soft-delete
// on the deleted_at column of the template_repository_configurations table.
// More info here: https://gorm.io/docs/many_to_many.html#Customize-JoinTable
func SetupGormTableOrFail(db *gorm.DB) {
	err := db.SetupJoinTable(models.Template{}, "RepositoryConfigurations", models.TemplateRepositoryConfiguration{})
	if err != nil {
		log.Logger.Fatal().Err(err).Msg("error setting up join table for templates_repository_configurations")
	}
}

//go:generate $GO_OUTPUT/mockery --name RepositoryConfigDao --filename repository_configs_mock.go --inpackage
type RepositoryConfigDao interface {
	Create(ctx context.Context, newRepo api.RepositoryRequest) (api.RepositoryResponse, error)
	BulkCreate(ctx context.Context, newRepositories []api.RepositoryRequest) ([]api.RepositoryResponse, []error)
	Update(ctx context.Context, orgID, uuid string, repoParams api.RepositoryUpdateRequest) (bool, error)
	Fetch(ctx context.Context, orgID string, uuid string) (api.RepositoryResponse, error)
	InternalOnly_ListReposToSnapshot(ctx context.Context, filter *ListRepoFilter) ([]models.RepositoryConfiguration, error)
	List(ctx context.Context, orgID string, paginationData api.PaginationData, filterData api.FilterData) (api.RepositoryCollectionResponse, int64, error)
	Delete(ctx context.Context, orgID string, uuid string) error
	SoftDelete(ctx context.Context, orgID string, uuid string) error
	BulkDelete(ctx context.Context, orgID string, uuids []string) []error
	SavePublicRepos(ctx context.Context, urls []string) error
	ValidateParameters(ctx context.Context, orgId string, params api.RepositoryValidationRequest, excludedUUIDS []string) (api.RepositoryValidationResponse, error)
	FetchByRepoUuid(ctx context.Context, orgID string, repoUuid string) (api.RepositoryResponse, error)
	InternalOnly_FetchRepoConfigsForRepoUUID(ctx context.Context, uuid string) []api.RepositoryResponse
	UpdateLastSnapshotTask(ctx context.Context, taskUUID string, orgID string, repoUUID string) error
	InternalOnly_RefreshRedHatRepo(ctx context.Context, request api.RepositoryRequest, label string) (*api.RepositoryResponse, error)
	FetchWithoutOrgID(ctx context.Context, uuid string) (api.RepositoryResponse, error)
	BulkExport(ctx context.Context, orgID string, reposToExport api.RepositoryExportRequest) ([]api.RepositoryExportResponse, error)
	BulkImport(ctx context.Context, reposToImport []api.RepositoryRequest) ([]api.RepositoryImportResponse, []error)
}

//go:generate $GO_OUTPUT/mockery --name RpmDao --filename rpms_mock.go --inpackage
type RpmDao interface {
	List(ctx context.Context, orgID string, uuidRepo string, limit int, offset int, search string, sortBy string) (api.RepositoryRpmCollectionResponse, int64, error)
	Search(ctx context.Context, orgID string, request api.ContentUnitSearchRequest) ([]api.SearchRpmResponse, error)
	SearchSnapshotRpms(ctx context.Context, orgId string, request api.SnapshotSearchRpmRequest) ([]api.SearchRpmResponse, error)
	ListSnapshotRpms(ctx context.Context, orgId string, snapshotUUIDs []string, search string, pageOpts api.PaginationData) ([]api.SnapshotRpm, int, error)
	DetectRpms(ctx context.Context, orgID string, request api.DetectRpmsRequest) (*api.DetectRpmsResponse, error)
	ListSnapshotErrata(ctx context.Context, orgId string, snapshotUUIDs []string, filters tangy.ErrataListFilters, pageOpts api.PaginationData) ([]api.SnapshotErrata, int, error)
	InsertForRepository(ctx context.Context, repoUuid string, pkgs []yum.Package) (int64, error)
	OrphanCleanup(ctx context.Context) error
	ListTemplateRpms(ctx context.Context, orgId string, templateUUID string, search string, pageOpts api.PaginationData) ([]api.SnapshotRpm, int, error)
	ListTemplateErrata(ctx context.Context, orgId string, templateUUID string, filters tangy.ErrataListFilters, pageOpts api.PaginationData) ([]api.SnapshotErrata, int, error)
}

//go:generate $GO_OUTPUT/mockery --name RepositoryDao --filename repositories_mock.go --inpackage
type RepositoryDao interface {
	FetchForUrl(ctx context.Context, url string) (Repository, error)
	ListForIntrospection(ctx context.Context, urls *[]string, force bool) ([]Repository, error)
	ListPublic(ctx context.Context, paginationData api.PaginationData, _ api.FilterData) (api.PublicRepositoryCollectionResponse, int64, error)
	Update(ctx context.Context, repo RepositoryUpdate) error
	FetchRepositoryRPMCount(ctx context.Context, repoUUID string) (int, error)
	OrphanCleanup(ctx context.Context) error
}

//go:generate $GO_OUTPUT/mockery --name SnapshotDao --filename snapshots_mock.go --inpackage
type SnapshotDao interface {
	Create(ctx context.Context, snap *models.Snapshot) error
	List(ctx context.Context, orgID string, repoConfigUuid string, paginationData api.PaginationData, filterData api.FilterData) (api.SnapshotCollectionResponse, int64, error)
	ListByTemplate(ctx context.Context, orgID string, template api.TemplateResponse, repositorySearch string, paginationData api.PaginationData) (api.SnapshotCollectionResponse, int64, error)
	FetchForRepoConfigUUID(ctx context.Context, repoConfigUUID string) ([]models.Snapshot, error)
	Delete(ctx context.Context, snapUUID string) error
	FetchLatestSnapshot(ctx context.Context, repoConfigUUID string) (api.SnapshotResponse, error)
	FetchLatestSnapshotModel(ctx context.Context, repoConfigUUID string) (models.Snapshot, error)
	FetchSnapshotsByDateAndRepository(ctx context.Context, orgID string, request api.ListSnapshotByDateRequest) (api.ListSnapshotByDateResponse, error)
	FetchSnapshotByVersionHref(ctx context.Context, repoConfigUUID string, versionHref string) (*api.SnapshotResponse, error)
	GetRepositoryConfigurationFile(ctx context.Context, orgID, snapshotUUID string, isLatest bool) (string, error)
	Fetch(ctx context.Context, uuid string) (api.SnapshotResponse, error)
	FetchSnapshotsModelByDateAndRepository(ctx context.Context, orgID string, request api.ListSnapshotByDateRequest) ([]models.Snapshot, error)
}

//go:generate $GO_OUTPUT/mockery --name MetricsDao --filename metrics_mock.go --inpackage
type MetricsDao interface {
	RepositoriesCount(ctx context.Context) int
	RepositoryConfigsCount(ctx context.Context) int
	RepositoriesIntrospectionCount(ctx context.Context, hours int, public bool) IntrospectionCount
	PublicRepositoriesFailedIntrospectionCount(ctx context.Context) int
	OrganizationTotal(ctx context.Context) int64
	PendingTasksAverageLatency(ctx context.Context) float64
	PendingTasksCount(ctx context.Context) int64
	PendingTasksOldestTask(ctx context.Context) float64
	RHReposSnapshotNotCompletedInLast36HoursCount(ctx context.Context) int64
}

//go:generate $GO_OUTPUT/mockery --name TaskInfoDao --filename task_info_mock.go --inpackage
type TaskInfoDao interface {
	Fetch(ctx context.Context, OrgID string, id string) (api.TaskInfoResponse, error)
	List(ctx context.Context, OrgID string, pageData api.PaginationData, filterData api.TaskInfoFilterData) (api.TaskInfoCollectionResponse, int64, error)
	FetchActiveTasks(ctx context.Context, orgID string, objectUUID string, taskTypes ...string) ([]string, error)
	Cleanup(ctx context.Context) error
}

//go:generate $GO_OUTPUT/mockery --name AdminTaskDao --filename admin_tasks_mock.go --inpackage
type AdminTaskDao interface {
	Fetch(ctx context.Context, id string) (api.AdminTaskInfoResponse, error)
	List(ctx context.Context, pageData api.PaginationData, filterData api.AdminTaskFilterData) (api.AdminTaskInfoCollectionResponse, int64, error)
}

//go:generate $GO_OUTPUT/mockery --name DomainDao --filename domain_dao_mock.go --inpackage
type DomainDao interface {
	FetchOrCreateDomain(ctx context.Context, orgId string) (string, error)
	Fetch(ctx context.Context, orgId string) (string, error)
}

//go:generate $GO_OUTPUT/mockery --name PackageGroupDao --filename package_groups_mock.go --inpackage
type PackageGroupDao interface {
	List(ctx context.Context, orgID string, uuidRepo string, limit int, offset int, search string, sortBy string) (api.RepositoryPackageGroupCollectionResponse, int64, error)
	Search(ctx context.Context, orgID string, request api.ContentUnitSearchRequest) ([]api.SearchPackageGroupResponse, error)
	InsertForRepository(ctx context.Context, repoUuid string, pkgGroups []yum.PackageGroup) (int64, error)
	OrphanCleanup(ctx context.Context) error
	SearchSnapshotPackageGroups(ctx context.Context, orgId string, request api.SnapshotSearchRpmRequest) ([]api.SearchPackageGroupResponse, error)
}

//go:generate $GO_OUTPUT/mockery --name EnvironmentDao --filename environments_mock.go --inpackage
type EnvironmentDao interface {
	List(ctx context.Context, orgID string, uuidRepo string, limit int, offset int, search string, sortBy string) (api.RepositoryEnvironmentCollectionResponse, int64, error)
	Search(ctx context.Context, orgID string, request api.ContentUnitSearchRequest) ([]api.SearchEnvironmentResponse, error)
	InsertForRepository(ctx context.Context, repoUuid string, environments []yum.Environment) (int64, error)
	OrphanCleanup(ctx context.Context) error
	SearchSnapshotEnvironments(ctx context.Context, orgId string, request api.SnapshotSearchRpmRequest) ([]api.SearchEnvironmentResponse, error)
}

//go:generate $GO_OUTPUT/mockery --name TemplateDao --filename templates_mock.go --inpackage
type TemplateDao interface {
	Create(ctx context.Context, templateRequest api.TemplateRequest) (api.TemplateResponse, error)
	Fetch(ctx context.Context, orgID string, uuid string, includeSoftDel bool) (api.TemplateResponse, error)
	InternalOnlyFetchByName(ctx context.Context, name string) (models.Template, error)
	List(ctx context.Context, orgID string, paginationData api.PaginationData, filterData api.TemplateFilterData) (api.TemplateCollectionResponse, int64, error)
	SoftDelete(ctx context.Context, orgID string, uuid string) error
	Delete(ctx context.Context, orgID string, uuid string) error
	ClearDeletedAt(ctx context.Context, orgID string, uuid string) error
	Update(ctx context.Context, orgID string, uuid string, templParams api.TemplateUpdateRequest) (api.TemplateResponse, error)
	GetRepoChanges(ctx context.Context, templateUUID string, newRepoConfigUUIDs []string) ([]string, []string, []string, []string, error)
	GetDistributionHref(ctx context.Context, templateUUID string, repoConfigUUID string) (string, error)
	UpdateDistributionHrefs(ctx context.Context, templateUUID string, repoUUIDs []string, snapshots []models.Snapshot, repoDistributionMap map[string]string) error
	DeleteTemplateRepoConfigs(ctx context.Context, templateUUID string, keepRepoConfigUUIDs []string) error
	UpdateLastUpdateTask(ctx context.Context, taskUUID string, orgID string, templateUUID string) error
	UpdateLastError(ctx context.Context, orgID string, templateUUID string, lastUpdateSnapshotError string) error
	SetEnvironmentCreated(ctx context.Context, templateUUID string) error
	UpdateSnapshots(ctx context.Context, templateUUID string, repoUUIDs []string, snapshots []models.Snapshot) error
	DeleteTemplateSnapshot(ctx context.Context, snapshotUUID string) error
}
