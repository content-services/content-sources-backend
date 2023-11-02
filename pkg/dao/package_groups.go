package dao

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/api"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/yummy/pkg/yum"
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

func (r packageGroupDaoImpl) Search(orgID string, request api.SearchSharedRepositoryEntityRequest) ([]api.SearchPackageGroupResponse, error) {
	// Retrieve the repository id list
	if orgID == "" {
		return nil, fmt.Errorf("orgID can not be an empty string")
	}
	// Verify length of URLs or UUIDs is greater than 1
	if err := checkRequestUrlAndUuids(request); err != nil {
		return nil, err
	}
	// Set to default request limit if null or request limit max (500) if greater than max
	request = checkRequestLimit(request)

	// FIXME 103 Once the URL stored in the database does not allow
	//           "/" tail characters, this could be removed
	urls := handleTailChars(request)
	uuids := request.UUIDs

	// These commands add an aggregate function (and remove it first if it already exists)
	// to aggregate and concatenate arrays of package lists and execute the select statement.
	// Using the raw SQL query feature to drop / create an aggregate function to handle arrays of
	// different sizes and execute the select statement with the ARRAY(SELECT DISTINCT UNNEST(...)) construct.

	dataResponse := []api.SearchPackageGroupResponse{}
	db := r.db.
		Exec(`DROP AGGREGATE IF EXISTS array_concat_agg(anycompatiblearray);`).
		Exec(`
			CREATE AGGREGATE array_concat_agg(anycompatiblearray) (
				SFUNC = array_cat,
				STYPE = anycompatiblearray
			);
		`).
		Raw(`
			SELECT DISTINCT ON (package_groups.name)
					package_groups.name AS package_group_name,
					package_groups.description,
					ARRAY(SELECT DISTINCT UNNEST(array_concat_agg(package_groups.package_list))) AS package_list
			FROM
					package_groups
			INNER JOIN
					repositories_package_groups ON repositories_package_groups.package_group_uuid = package_groups.uuid
			INNER JOIN
					repositories ON repositories.uuid = repositories_package_groups.repository_uuid
			LEFT JOIN
					repository_configurations ON repository_configurations.repository_uuid = repositories.uuid
			WHERE
					(repository_configurations.org_id = ? OR repositories.public)
					AND package_groups.name ILIKE ?
					AND (repositories.url IN ? OR repository_configurations.uuid IN ?)
			GROUP BY
					package_groups.name, package_groups.description
			ORDER BY
					package_groups.name ASC
			LIMIT ?;
		`, orgID, fmt.Sprintf("%%%s%%", request.Search), urls, UuidifyStrings(uuids), *request.Limit).
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
		existingHashes []string
	)

	// Retrieve Repository record
	if repo, err = r.fetchRepo(repoUuid); err != nil {
		return 0, fmt.Errorf("failed to fetchRepo: %w", err)
	}

	// Build the lists of ids, names, and package lists from the package groups and generate a hash for each
	ids := make([]string, len(pkgGroups))
	names := make([]string, len(pkgGroups))
	packageLists := make([][]string, len(pkgGroups))
	hashValues := make([]string, len(pkgGroups))
	for i := 0; i < len(pkgGroups); i++ {
		ids[i] = pkgGroups[i].ID
		names[i] = string(pkgGroups[i].Name)
		packageLists[i] = pkgGroups[i].PackageList
		concatenatedString := concatenateStrings(ids[i], names[i], packageLists[i])
		hash := generateHash(concatenatedString)
		hashValues = append(hashValues, hash)
	}

	// Given the list of hashes, retrieve the list of the ones that exists
	// in the 'package_groups' table (whatever is the repository that it could belong)
	if err = r.db.
		Where("hash_value in (?)", hashValues).
		Model(&models.PackageGroup{}).
		Pluck("hash_value", &existingHashes).Error; err != nil {
		return 0, fmt.Errorf("failed retrieving existing id in package_groups: %w", err)
	}

	// Given a slice of yum.PackageGroup, it converts the groups
	// to the model and returns a slice of models.PackageGroup
	dbPkgGroups := FilteredConvertPackageGroups(pkgGroups, existingHashes)

	// Insert the filtered package groups in package_groups table
	result := r.db.Create(dbPkgGroups)
	if result.Error != nil {
		return 0, fmt.Errorf("failed to PagedPackageGroupInsert: %w", err)
	}

	// Now fetch the uuids of all the package groups we want associated to the repository
	var pkgGroupUuids []string
	if err = r.db.
		Where("hash_value in (?)", hashValues).
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
// while filtering out any groups with hashes in the excludedHashes parameter
func FilteredConvertPackageGroups(yumPkgGroups []yum.PackageGroup, excludedHashes []string) []models.PackageGroup {
	var dbPkgGroups []models.PackageGroup
	for _, yumPkgGroup := range yumPkgGroups {
		concatenatedString := concatenateStrings(yumPkgGroup.ID, string(yumPkgGroup.Name), yumPkgGroup.PackageList)
		hash := generateHash(concatenatedString)
		if !stringInSlice(hash, excludedHashes) {
			dbPkgGroups = append(dbPkgGroups, models.PackageGroup{
				ID:          yumPkgGroup.ID,
				Name:        string(yumPkgGroup.Name),
				Description: string(yumPkgGroup.Description),
				PackageList: yumPkgGroup.PackageList,
				HashValue:   hash,
			})
		}
	}
	return dbPkgGroups
}

// concatenateStrings given a variable number of arguments of any type, concatenates multiple strings into a single string
func concatenateStrings(strings ...interface{}) string {
	var concatenatedString string
	for _, str := range strings {
		concatenatedString += fmt.Sprint(str)
	}
	return concatenatedString
}

// generateHash generates a SHA-256 hash from a string
func generateHash(input string) string {
	hasher := sha256.New()
	hasher.Write([]byte(input))
	hashBytes := hasher.Sum(nil)
	return hex.EncodeToString(hashBytes)
}
