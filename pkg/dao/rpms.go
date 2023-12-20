package dao

import (
	"context"
	"fmt"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/yummy/pkg/yum"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type rpmDaoImpl struct {
	db *gorm.DB
}

func GetRpmDao(db *gorm.DB) RpmDao {
	// Return DAO instance
	return &rpmDaoImpl{
		db: db,
	}
}

func (r *rpmDaoImpl) List(
	orgID string,
	repositoryConfigUUID string,
	limit int, offset int,
	search string,
	sortBy string,
) (api.RepositoryRpmCollectionResponse, int64, error) {
	// Check arguments
	if orgID == "" {
		return api.RepositoryRpmCollectionResponse{}, 0, fmt.Errorf("orgID can not be an empty string")
	}

	var totalRpms int64
	repoRpms := []models.Rpm{}

	if ok, err := isOwnedRepository(r.db, orgID, repositoryConfigUUID); !ok {
		if err != nil {
			return api.RepositoryRpmCollectionResponse{},
				totalRpms,
				DBErrorToApi(err)
		}
		return api.RepositoryRpmCollectionResponse{},
			totalRpms,
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
		return api.RepositoryRpmCollectionResponse{}, totalRpms, err
	}

	filteredDB := r.db.Model(&repoRpms).Joins(strings.Join([]string{"inner join", models.TableNameRpmsRepositories, "on uuid = rpm_uuid"}, " ")).
		Where("repository_uuid = ?", repositoryConfig.Repository.UUID)

	if search != "" {
		containsSearch := "%" + search + "%"
		filteredDB = filteredDB.
			Where("name LIKE ?", containsSearch)
	}

	sortMap := map[string]string{
		"name":    "name",
		"release": "release",
		"version": "version",
		"arch":    "arch",
	}

	order := convertSortByToSQL(sortBy, sortMap, "name asc")

	filteredDB = filteredDB.
		Order(order).
		Count(&totalRpms).
		Offset(offset).
		Limit(limit).
		Find(&repoRpms)

	if filteredDB.Error != nil {
		return api.RepositoryRpmCollectionResponse{}, totalRpms, filteredDB.Error
	}

	// Return the rpm list
	repoRpmResponse := r.RepositoryRpmListFromModelToResponse(repoRpms)
	return api.RepositoryRpmCollectionResponse{
		Data: repoRpmResponse,
		Meta: api.ResponseMetadata{
			Count:  totalRpms,
			Offset: offset,
			Limit:  limit,
		},
	}, totalRpms, nil
}

func (r *rpmDaoImpl) RepositoryRpmListFromModelToResponse(repoRpm []models.Rpm) []api.RepositoryRpm {
	repos := make([]api.RepositoryRpm, len(repoRpm))
	for i := 0; i < len(repoRpm); i++ {
		r.modelToApiFields(&repoRpm[i], &repos[i])
	}
	return repos
}

// apiFieldsToModel transform from database model to API request.
// in the source models.Rpm structure.
// out the output api.RepositoryRpm structure.
//
// NOTE: This encapsulate transformation into rpmDaoImpl implementation
// as the methods are not used outside; if they were used
// out of this place, decouple into a new struct and make
// he methods publics.
func (r *rpmDaoImpl) modelToApiFields(in *models.Rpm, out *api.RepositoryRpm) {
	if in == nil || out == nil {
		return
	}
	out.UUID = in.Base.UUID
	out.Name = in.Name
	out.Arch = in.Arch
	out.Version = in.Version
	out.Release = in.Release
	out.Epoch = in.Epoch
	out.Summary = in.Summary
	out.Checksum = in.Checksum
}

func (r rpmDaoImpl) Search(orgID string, request api.ContentUnitSearchRequest) ([]api.SearchRpmResponse, error) {
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

	// This implement the following SELECT statement:
	//
	// SELECT DISTINCT ON (rpms.name)
	//        rpms.name, rpms.summary
	// FROM rpms
	//      inner join repositories_rpms on repositories_rpms.rpm_uuid = rpms.uuid
	//      inner join repositories on repositories.uuid = repositories_rpms.repository_uuid
	//      left join repository_configurations on repository_configurations.repository_uuid = repositories.uuid
	// WHERE (repository_configurations.org_id = 'acme' OR repositories.public)
	//       AND ( repositories.url in (...)
	//             OR repository_configurations.uuid in (...)
	//       )
	//       AND rpms.name LIKE 'demo%'
	// ORDER BY rpms.name, rpms.epoch DESC
	// LIMIT 20;

	// https://github.com/go-gorm/gorm/issues/5318
	dataResponse := []api.SearchRpmResponse{}
	orGroupPublicOrPrivate := r.db.Where("repository_configurations.org_id = ?", orgID).Or("repositories.public")
	db := r.db.
		Select("DISTINCT ON(rpms.name) rpms.name as package_name", "rpms.summary").
		Table(models.TableNameRpm).
		Joins("inner join repositories_rpms on repositories_rpms.rpm_uuid = rpms.uuid").
		Joins("inner join repositories on repositories.uuid = repositories_rpms.repository_uuid").
		Joins("left join repository_configurations on repository_configurations.repository_uuid = repositories.uuid").
		Where(orGroupPublicOrPrivate).
		Where("rpms.name ILIKE ?", fmt.Sprintf("%%%s%%", request.Search)).
		Where(r.db.Where("repositories.url in ?", urls).
			Or("repository_configurations.uuid in ?", UuidifyStrings(uuids))).
		Order("rpms.name ASC").
		Limit(*request.Limit).
		Scan(&dataResponse)

	if db.Error != nil {
		return nil, db.Error
	}

	return dataResponse, nil
}

func (r *rpmDaoImpl) fetchRepo(uuid string) (models.Repository, error) {
	found := models.Repository{}
	if err := r.db.
		Where("UUID = ?", uuid).
		First(&found).
		Error; err != nil {
		return found, err
	}
	return found, nil
}

// InsertForRepository inserts a set of yum packages for a given repository
// and removes any that are not in the list.  This will involve inserting the RPMs
// if not present, and adding or removing any associations to the Repository
// Returns a count of new RPMs added to the system (not the repo), as well as any error
func (r *rpmDaoImpl) InsertForRepository(repoUuid string, pkgs []yum.Package) (int64, error) {
	var (
		err               error
		repo              models.Repository
		existingChecksums []string
	)

	// Retrieve Repository record
	if repo, err = r.fetchRepo(repoUuid); err != nil {
		return 0, fmt.Errorf("failed to fetchRepo: %w", err)
	}

	// Build the list of checksums from the provided packages
	checksums := make([]string, len(pkgs))
	for i := 0; i < len(pkgs); i++ {
		checksums[i] = pkgs[i].Checksum.Value
	}

	// Given the list of checksums, retrieve the list of the ones that exists
	// in the 'rpm' table (whatever is the repository that it could belong)
	if err = r.db.
		Where("checksum in (?)", checksums).
		Model(&models.Rpm{}).
		Pluck("checksum", &existingChecksums).Error; err != nil {
		return 0, fmt.Errorf("failed retrieving existing checksum in rpms: %w", err)
	}

	// Given a slice of yum.Package, it filters the ones which checksum exists
	// in existingChecksums and return a slice of models.Rpm
	dbPkgs := FilteredConvert(pkgs, existingChecksums)

	// Insert the filtered packages in rpms table
	result := r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "checksum"}},
		DoNothing: true,
	}).Create(dbPkgs)
	if result.Error != nil {
		return 0, fmt.Errorf("failed to PagedRpmInsert: %w", err)
	}

	// Now fetch the uuids of all the rpms we want associated to the repository
	var rpmUuids []string
	if err = r.db.
		Where("checksum in (?)", checksums).
		Model(&models.Rpm{}).
		Pluck("uuid", &rpmUuids).Error; err != nil {
		return 0, fmt.Errorf("failed retrieving rpms.uuid for the package checksums: %w", err)
	}

	// Delete Rpm and RepositoryRpm entries we don't need
	if err = r.deleteUnneeded(repo, rpmUuids); err != nil {
		return 0, fmt.Errorf("failed to deleteUnneeded: %w", err)
	}

	// Add the RepositoryRpm entries we do need
	associations := prepRepositoryRpms(repo, rpmUuids)
	result = r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "repository_uuid"}, {Name: "rpm_uuid"}},
		DoNothing: true}).
		Create(&associations)
	if result.Error != nil {
		return result.RowsAffected, fmt.Errorf("failed to Create: %w", result.Error)
	}

	return result.RowsAffected, err
}

// prepRepositoryRpms  converts a list of rpm_uuids to a list of RepositoryRpm Objects
func prepRepositoryRpms(repo models.Repository, rpm_uuids []string) []models.RepositoryRpm {
	repoRpms := make([]models.RepositoryRpm, len(rpm_uuids))
	for i := 0; i < len(rpm_uuids); i++ {
		repoRpms[i].RepositoryUUID = repo.UUID
		repoRpms[i].RpmUUID = rpm_uuids[i]
	}
	return repoRpms
}

// difference returns the difference between arrays a and b   (a - b)
func difference(a, b []string) []string {
	mb := make(map[string]struct{}, len(b))
	for _, x := range b {
		mb[x] = struct{}{}
	}
	var diff []string
	for _, x := range a {
		if _, found := mb[x]; !found {
			diff = append(diff, x)
		}
	}
	return diff
}

// deleteUnneeded Removes any RepositoryRpm entries that are not in the list of rpm_uuids
func (r *rpmDaoImpl) deleteUnneeded(repo models.Repository, rpm_uuids []string) error {
	// First get uuids that are there:
	var (
		existing_rpm_uuids []string
	)

	// Read existing rpm_uuid associated to repository_uuid
	if err := r.db.Model(&models.RepositoryRpm{}).
		Where("repository_uuid = ?", repo.UUID).
		Pluck("rpm_uuid", &existing_rpm_uuids).
		Error; err != nil {
		return err
	}

	rpmsToDelete := difference(existing_rpm_uuids, rpm_uuids)

	// Delete the many2many relationship for the unneeded rpms
	if err := r.db.
		Unscoped().
		Where("repositories_rpms.repository_uuid = ?", repo.UUID).
		Where("repositories_rpms.rpm_uuid in (?)", rpmsToDelete).
		Delete(&models.RepositoryRpm{}).
		Error; err != nil {
		return err
	}

	return nil
}

func (r *rpmDaoImpl) OrphanCleanup() error {
	var danglingRpmUuids []string

	// Retrieve dangling rpms.uuid
	if err := r.db.
		Model(&models.Rpm{}).
		Where("repositories_rpms.rpm_uuid is NULL").
		Joins("left join repositories_rpms on rpms.uuid = repositories_rpms.rpm_uuid").
		Pluck("rpms.uuid", &danglingRpmUuids).
		Error; err != nil {
		return err
	}

	if len(danglingRpmUuids) == 0 {
		return nil
	}

	// Remove dangling rpms
	if err := r.db.
		Where("rpms.uuid in (?)", danglingRpmUuids).
		Delete(&models.Rpm{}).
		Error; err != nil {
		return err
	}
	return nil
}

func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

// FilteredConvert Given a list of yum.Package objects, it converts them to model.Rpm packages
// while filtering out any checksums that are in the excludedChecksums parameter
func FilteredConvert(yumPkgs []yum.Package, excludeChecksums []string) []models.Rpm {
	var dbPkgs []models.Rpm
	for _, yumPkg := range yumPkgs {
		if !stringInSlice(yumPkg.Checksum.Value, excludeChecksums) {
			epoch := yumPkg.Version.Epoch
			dbPkgs = append(dbPkgs, models.Rpm{
				Name:     yumPkg.Name,
				Arch:     yumPkg.Arch,
				Version:  yumPkg.Version.Version,
				Release:  yumPkg.Version.Release,
				Epoch:    epoch,
				Checksum: yumPkg.Checksum.Value,
				Summary:  yumPkg.Summary,
			})
		}
	}
	return dbPkgs
}

func (r *rpmDaoImpl) SearchSnapshotRpms(ctx context.Context, orgId string, request api.SnapshotSearchRpmRequest) ([]api.SearchRpmResponse, error) {
	response := []api.SearchRpmResponse{}

	pulpHrefs := []string{}
	res := readableSnapshots(r.db, orgId).Where("snapshots.UUID in ?", UuidifyStrings(request.UUIDs)).Pluck("version_href", &pulpHrefs)
	if res.Error != nil {
		return response, fmt.Errorf("failed to query the db for snapshots %w", res.Error)
	}
	if config.Tang == nil {
		return response, fmt.Errorf("no tang configuration present")
	}

	if len(pulpHrefs) == 0 {
		return response, nil
	}

	pkgs, err := (*config.Tang).RpmRepositoryVersionPackageSearch(ctx, pulpHrefs, request.Search, *request.Limit)
	if err != nil {
		return response, fmt.Errorf("error querying packages in snapshots %w", err)
	}
	for _, pkg := range pkgs {
		response = append(response, api.SearchRpmResponse{
			PackageName: pkg.Name,
			Summary:     pkg.Summary,
		})
	}
	return response, nil
}
