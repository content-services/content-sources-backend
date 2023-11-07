package dao

import (
	"fmt"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/api"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/yummy/pkg/yum"
	"github.com/openlyinc/pointy"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type packageGroupDaoImpl struct {
	db *gorm.DB
}

func GetPackageGroupDao(db *gorm.DB) PackageGroupDao {
	// Return DAO instance
	return packageGroupDaoImpl{
		db: db,
	}
}

func (r packageGroupDaoImpl) isOwnedRepository(orgID string, repositoryConfigUUID string) (bool, error) {
	var repoConfigs []models.RepositoryConfiguration
	var count int64
	if err := r.db.
		Where("org_id = ? and uuid = ?", orgID, UuidifyString(repositoryConfigUUID)).
		Find(&repoConfigs).
		Count(&count).
		Error; err != nil {
		return false, err
	}
	if count == 0 {
		return false, nil
	}
	return true, nil
}

func (r packageGroupDaoImpl) List(orgID string, repositoryConfigUUID string, limit int, offset int, search string, sortBy string) (api.RepositoryPackageGroupCollectionResponse, int64, error) {
	// Check arguments
	if orgID == "" {
		return api.RepositoryPackageGroupCollectionResponse{}, 0, fmt.Errorf("orgID can not be an empty string")
	}

	var totalPackageGroups int64
	repoPackageGroups := []models.PackageGroup{}

	if ok, err := r.isOwnedRepository(orgID, repositoryConfigUUID); !ok {
		if err != nil {
			return api.RepositoryPackageGroupCollectionResponse{},
				totalPackageGroups,
				DBErrorToApi(err)
		}
		return api.RepositoryPackageGroupCollectionResponse{},
			totalPackageGroups,
			&ce.DaoError{
				NotFound: true,
				Message:  "Could not find repository with UUID " + repositoryConfigUUID,
			}
	}

	repositoryConfig := models.RepositoryConfiguration{}
	// Select Repository from RepositoryConfig

	if err := r.db.
		Preload("Repository").
		Find(&repositoryConfig, "uuid = ?", repositoryConfigUUID).
		Error; err != nil {
		return api.RepositoryPackageGroupCollectionResponse{}, totalPackageGroups, err
	}

	filteredDB := r.db.Model(&repoPackageGroups).Joins(strings.Join([]string{"inner join", models.TableNamePackageGroupsRepositories, "on uuid = package_group_uuid"}, " ")).
		Where("repository_uuid = ?", repositoryConfig.Repository.UUID)

	if search != "" {
		containsSearch := "%" + search + "%"
		filteredDB = filteredDB.
			Where("name LIKE ?", containsSearch)
	}

	sortMap := map[string]string{
		"id":          "id",
		"name":        "name",
		"description": "description",
		"packagelist": "packagelist",
	}

	order := convertSortByToSQL(sortBy, sortMap, "name asc")

	filteredDB = filteredDB.
		Order(order).
		Count(&totalPackageGroups).
		Offset(offset).
		Limit(limit).
		Find(&repoPackageGroups)

	if filteredDB.Error != nil {
		return api.RepositoryPackageGroupCollectionResponse{}, totalPackageGroups, filteredDB.Error
	}

	// Return the package group list
	repoPackageGroupResponse := r.RepositoryPackageGroupListFromModelToResponse(repoPackageGroups)
	return api.RepositoryPackageGroupCollectionResponse{
		Data: repoPackageGroupResponse,
		Meta: api.ResponseMetadata{
			Count:  totalPackageGroups,
			Offset: offset,
			Limit:  limit,
		},
	}, totalPackageGroups, nil
}

func (r packageGroupDaoImpl) RepositoryPackageGroupListFromModelToResponse(repoPackageGroup []models.PackageGroup) []api.RepositoryPackageGroup {
	repos := make([]api.RepositoryPackageGroup, len(repoPackageGroup))
	for i := 0; i < len(repoPackageGroup); i++ {
		r.modelToApiFields(&repoPackageGroup[i], &repos[i])
	}
	return repos
}

// apiFieldsToModel transform from database model to API request.
// in the source models.PackageGroup structure.
// out the output api.RepositoryPackageGroup structure.
//
// NOTE: This encapsulate transformation into packageGroupDaoImpl implementation
// as the methods are not used outside; if they were used
// out of this place, decouple into a new struct and make
// he methods publics.
func (r packageGroupDaoImpl) modelToApiFields(in *models.PackageGroup, out *api.RepositoryPackageGroup) {
	if in == nil || out == nil {
		return
	}
	out.UUID = in.Base.UUID
	out.ID = in.ID
	out.Name = in.Name
	out.Description = in.Description
	out.PackageList = in.PackageList
}

func (r packageGroupDaoImpl) Search(orgID string, request api.SearchPackageGroupRequest) ([]api.SearchPackageGroupResponse, error) {
	// Retrieve the repository id list
	if orgID == "" {
		return nil, fmt.Errorf("orgID can not be an empty string")
	}
	if len(request.URLs) == 0 && len(request.UUIDs) == 0 {
		return nil, fmt.Errorf("must contain at least 1 URL or 1 UUID")
	}
	if request.Limit == nil {
		request.Limit = pointy.Int(api.SearchPackageGroupRequestLimitDefault)
	}
	if *request.Limit > api.SearchPackageGroupRequestLimitMaximum {
		request.Limit = pointy.Int(api.SearchPackageGroupRequestLimitMaximum)
	}

	// FIXME 103 Once the URL stored in the database does not allow
	//           "/" tail characters, this could be removed
	urls := make([]string, len(request.URLs)*2)
	for i, url := range request.URLs {
		urls[i*2] = url
		urls[i*2+1] = url + "/"
	}
	uuids := request.UUIDs

	// This implements the following SELECT statement:
	//
	// SELECT DISTINCT ON (package_groups.name)
	//        package_groups.name, package_groups.description
	// FROM package_groups
	//      inner join repositories_package_groups on repositories_package_groups.package_group_uuid = package_groups.uuid
	//      inner join repositories on repositories.uuid = repositories_package_groups.repository_uuid
	//      left join repository_configurations on repository_configurations.repository_uuid = repositories.uuid
	// WHERE (repository_configurations.org_id = 'acme' OR repositories.public)
	//       AND ( repositories.url in (...)
	//             OR repository_configurations.uuid in (...)
	//       )
	//       AND package_groups.name LIKE 'demo%'
	// ORDER BY package_groups.name DESC
	// LIMIT 20;

	// https://github.com/go-gorm/gorm/issues/5318
	dataResponse := []api.SearchPackageGroupResponse{}
	orGroupPublicOrPrivate := r.db.Where("repository_configurations.org_id = ?", orgID).Or("repositories.public")
	db := r.db.
		Select("DISTINCT ON(package_groups.name) package_groups.name as package_group_name", "package_groups.description").
		Table(models.TableNamePackageGroup).
		Joins("inner join repositories_package_groups on repositories_package_groups.package_group_uuid = package_groups.uuid").
		Joins("inner join repositories on repositories.uuid = repositories_package_groups.repository_uuid").
		Joins("left join repository_configurations on repository_configurations.repository_uuid = repositories.uuid").
		Where(orGroupPublicOrPrivate).
		Where("package_groups.name ILIKE ?", fmt.Sprintf("%%%s%%", request.Search)).
		Where(r.db.Where("repositories.url in ?", urls).
			Or("repository_configurations.uuid in ?", UuidifyStrings(uuids))).
		Order("package_groups.name ASC").
		Limit(*request.Limit).
		Scan(&dataResponse)

	if db.Error != nil {
		return nil, db.Error
	}

	return dataResponse, nil
}

func (r packageGroupDaoImpl) fetchRepo(uuid string) (models.Repository, error) {
	found := models.Repository{}
	if err := r.db.
		Where("UUID = ?", uuid).
		First(&found).
		Error; err != nil {
		return found, err
	}
	return found, nil
}

// InsertForRepository inserts a set of yum package groups for a given repository
// and removes any that are not in the list.  This will involve inserting the package groups
// if not present, and adding or removing any associations to the Repository
// Returns a count of new package groups added to the system (not the repo), as well as any error
func (r packageGroupDaoImpl) InsertForRepository(repoUuid string, pkgGroups []yum.PackageGroup) (int64, error) {
	var (
		err            error
		repo           models.Repository
		existingGroups []string
	)

	// Retrieve Repository record
	if repo, err = r.fetchRepo(repoUuid); err != nil {
		return 0, fmt.Errorf("failed to fetchRepo: %w", err)
	}

	// Build list of ids and names to deduplicate on
	ids := make([]string, len(pkgGroups))
	names := make([]string, len(pkgGroups))
	for i := 0; i < len(pkgGroups); i++ {
		ids[i] = pkgGroups[i].ID
		names[i] = string(pkgGroups[i].Name)
	}

	// Given the list of ids and names, retrieve the list of the ones that exists
	// in the 'package_groups' table (whatever is the repository that it could belong)
	if err = r.db.
		Where("id in (?)", ids).
		Where("name in (?)", names).
		Model(&models.PackageGroup{}).
		Pluck("id", &existingGroups).Error; err != nil {
		return 0, fmt.Errorf("failed retrieving existing id in package_groups: %w", err)
	}

	// Given a slice of yum.PackageGroup, it filters the groups
	// in existingGroups and return a slice of models.PackageGroup
	dbPkgGroups := FilteredConvertPackageGroups(pkgGroups, existingGroups)

	// 	// Insert the filtered package groups in package_groups table
	result := r.db.Create(dbPkgGroups)
	if result.Error != nil {
		return 0, fmt.Errorf("failed to PagedPackageGroupInsert: %w", err)
	}

	// Now fetch the uuids of all the package groups we want associated to the repository
	var pkgGroupUuids []string
	if err = r.db.
		Where("id in (?)", ids).
		Where("name in (?)", names).
		Model(&models.PackageGroup{}).
		Pluck("uuid", &pkgGroupUuids).Error; err != nil {
		return 0, fmt.Errorf("failed retrieving package_groups.uuid for the given package groups: %w", err)
	}

	// Delete PackageGroup and RepositoryPackageGroup entries we don't need
	if err = r.deleteUnneeded(repo, pkgGroupUuids); err != nil {
		return 0, fmt.Errorf("failed to deleteUnneeded: %w", err)
	}

	// Add the RepositoryPackageGroup entries we do need
	associations := prepRepositoryPackageGroups(repo, pkgGroupUuids)
	result = r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "repository_uuid"}, {Name: "package_group_uuid"}},
		DoNothing: true}).
		Create(&associations)
	if result.Error != nil {
		return result.RowsAffected, fmt.Errorf("failed to Create: %w", result.Error)
	}

	return result.RowsAffected, err
}

// prepRepositoryPackageGroups converts a list of package_group_uuids to a list of RepositoryPackageGroup Objects
func prepRepositoryPackageGroups(repo models.Repository, package_group_uuids []string) []models.RepositoryPackageGroup {
	repoPackageGroups := make([]models.RepositoryPackageGroup, len(package_group_uuids))
	for i := 0; i < len(package_group_uuids); i++ {
		repoPackageGroups[i].RepositoryUUID = repo.UUID
		repoPackageGroups[i].PackageGroupUUID = package_group_uuids[i]
	}
	return repoPackageGroups
}

// deleteUnneeded removes any RepositoryPackageGroup entries that are not in the list of package_group_uuids
func (r packageGroupDaoImpl) deleteUnneeded(repo models.Repository, package_group_uuids []string) error {
	// First get uuids that are there:
	var (
		existing_package_group_uuids []string
	)

	// Read existing package_group_uuid associated to repository_uuid
	if err := r.db.Model(&models.RepositoryPackageGroup{}).
		Where("repository_uuid = ?", repo.UUID).
		Pluck("package_group_uuid", &existing_package_group_uuids).
		Error; err != nil {
		return err
	}

	packageGroupsToDelete := difference(existing_package_group_uuids, package_group_uuids)

	// Delete the many2many relationship for the unneeded package groups
	if err := r.db.
		Unscoped().
		Where("repositories_package_groups.repository_uuid = ?", repo.UUID).
		Where("repositories_package_groups.package_group_uuid in (?)", packageGroupsToDelete).
		Delete(&models.RepositoryPackageGroup{}).
		Error; err != nil {
		return err
	}

	return nil
}

func (r packageGroupDaoImpl) OrphanCleanup() error {
	var danglingPackageGroupUuids []string

	// Retrieve dangling package_groups.uuid
	if err := r.db.
		Model(&models.PackageGroup{}).
		Where("repositories_package_groups.package_group_uuid is NULL").
		Joins("left join repositories_package_groups on package_groups.uuid = repositories_package_groups.package_group_uuid").
		Pluck("package_groups.uuid", &danglingPackageGroupUuids).
		Error; err != nil {
		return err
	}

	if len(danglingPackageGroupUuids) == 0 {
		return nil
	}

	// Remove dangling package groups
	if err := r.db.
		Where("package_groups.uuid in (?)", danglingPackageGroupUuids).
		Delete(&models.PackageGroup{}).
		Error; err != nil {
		return err
	}
	return nil
}

// FilteredConvertPackageGroups Given a list of yum.PackageGroup objects, it converts them to model.PackageGroup
// while filtering out any groups that are in the excludedGroups parameter
func FilteredConvertPackageGroups(yumPkgGroups []yum.PackageGroup, excludedGroups []string) []models.PackageGroup {
	var dbPkgGroups []models.PackageGroup
	for _, yumPkgGroup := range yumPkgGroups {
		if !stringInSlice(string(yumPkgGroup.ID), excludedGroups) {
			dbPkgGroups = append(dbPkgGroups, models.PackageGroup{
				ID:          yumPkgGroup.ID,
				Name:        string(yumPkgGroup.Name),
				Description: string(yumPkgGroup.Description),
				PackageList: yumPkgGroup.PackageList,
			})
		}
	}
	return dbPkgGroups
}
