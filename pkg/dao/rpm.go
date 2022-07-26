package dao

import (
	"fmt"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/yummy/pkg/yum"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const defaultPagedRpmInsertsLimit = 100

const OptionPagedRpmInsertsLimit = "pagedRpmInsertsLimit"

type rpmDaoImpl struct {
	db                   *gorm.DB
	pagedRpmInsertsLimit int
}

func GetRpmDao(db *gorm.DB, options map[string]interface{}) RpmDao {
	var (
		pagedRpmInsertsLimit int = defaultPagedRpmInsertsLimit
	)
	// Read pagedRpmInsertsLimit option
	if value, ok := options[OptionPagedRpmInsertsLimit]; ok {
		if value, ok := value.(int); ok {
			pagedRpmInsertsLimit = value
		}
	}
	return rpmDaoImpl{
		db:                   db,
		pagedRpmInsertsLimit: pagedRpmInsertsLimit,
	}
}

func (r rpmDaoImpl) isOwnedRepository(orgID string, repositoryConfigUUID string) (bool, error) {
	var repoConfigs []models.RepositoryConfiguration
	var count int64
	if err := r.db.
		Where("org_id = ? and uuid = ?", orgID, repositoryConfigUUID).
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

func (r rpmDaoImpl) List(orgID string, repositoryConfigUUID string, limit int, offset int) (api.RepositoryRpmCollectionResponse, int64, error) {
	// Check arguments
	if orgID == "" {
		return api.RepositoryRpmCollectionResponse{}, 0, fmt.Errorf("orgID can not be an empty string")
	}

	var totalRpms int64
	repoRpms := []models.Rpm{}

	if ok, err := r.isOwnedRepository(orgID, repositoryConfigUUID); !ok {
		if err != nil {
			return api.RepositoryRpmCollectionResponse{},
				totalRpms,
				DBErrorToApi(err)
		}
		return api.RepositoryRpmCollectionResponse{},
			totalRpms,
			fmt.Errorf("repositoryConfigUUID = %s is not owned", repositoryConfigUUID)
	}

	repositoryConfig := models.RepositoryConfiguration{}
	// Select Repository from RepositoryConfig
	if err := r.db.
		Preload("Repository").
		Find(&repositoryConfig, "uuid = ?", repositoryConfigUUID).
		Error; err != nil {
		return api.RepositoryRpmCollectionResponse{}, totalRpms, err
	}
	if err := r.db.
		Model(&repoRpms).
		Joins(strings.Join([]string{"inner join", models.TableNameRpmsRepositories, "on uuid = rpm_uuid"}, " ")).
		Where("repository_uuid = ?", repositoryConfig.Repository.UUID).
		Count(&totalRpms).
		Offset(offset).
		Limit(limit).
		Find(&repoRpms).
		Error; err != nil {
		return api.RepositoryRpmCollectionResponse{}, totalRpms, err
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

func (r rpmDaoImpl) RepositoryRpmListFromModelToResponse(repoRpm []models.Rpm) []api.RepositoryRpm {
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
func (r rpmDaoImpl) modelToApiFields(in *models.Rpm, out *api.RepositoryRpm) {
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

func (r rpmDaoImpl) Search(orgID string, request api.SearchRpmRequest, limit int) ([]api.SearchRpmResponse, error) {
	// Retrieve the repository id list
	if orgID == "" {
		return nil, fmt.Errorf("orgID can not be an empty string")
	}
	if len(request.URLs) == 0 {
		return nil, fmt.Errorf("request.URLs must contain at least 1 URL")
	}

	// FIXME 103 Once the URL stored in the database does not allow
	//           "/" tail characters, this could be removed
	urls := make([]string, len(request.URLs)*2)
	for i, url := range request.URLs {
		urls[i*2] = url
		urls[i*2+1] = url + "/"
	}

	// This implement the following SELECT statement:
	//
	// SELECT DISTINCT ON (rpms.name)
	//        rpms.name, rpms.summary
	// FROM rpms
	//      inner join repositories_rpms on repositories_rpms.rpm_uuid = rpms.uuid
	//      inner join repositories on repositories.uuid = repositories_rpms.repository_uuid
	//      left join repository_configurations on repository_configurations.repository_uuid = repositories.uuid
	// WHERE (repository_configurations.org_id = 'acme' OR repositories.public)
	//       AND repositories.public
	//       AND rpms.name LIKE 'demo%'
	// ORDER BY rpms.name, rpms.epoch DESC
	// LIMIT 20;

	// https://github.com/go-gorm/gorm/issues/5318
	dataResponse := []api.SearchRpmResponse{}
	orGroup := r.db.Where("repository_configurations.org_id = ?", orgID).Or("repositories.public")
	db := r.db.
		Select("DISTINCT ON(rpms.name) rpms.name as package_name", "rpms.summary").
		Table(models.TableNameRpm).
		Joins("inner join repositories_rpms on repositories_rpms.rpm_uuid = rpms.uuid").
		Joins("inner join repositories on repositories.uuid = repositories_rpms.repository_uuid").
		Joins("left join repository_configurations on repository_configurations.repository_uuid = repositories.uuid").
		Where(orGroup).
		Where("rpms.name LIKE ?", fmt.Sprintf("%s%%", request.Search)).
		Where("repositories.url in ?", urls).
		Order("rpms.name ASC").
		Limit(limit).
		Scan(&dataResponse)

	if db.Error != nil {
		return nil, db.Error
	}

	return dataResponse, nil
}

// PagedRpmInsert insert all passed in rpms quickly, ignoring any duplicates
// Returns count of new packages inserted, and any errors
func (r rpmDaoImpl) PagedRpmInsert(pkgs *[]models.Rpm) (int64, error) {
	var count int64
	chunk := r.pagedRpmInsertsLimit
	var result *gorm.DB
	if len(*pkgs) == 0 {
		return 0, nil
	}

	for i := 0; i < len(*pkgs); i += chunk {
		end := i + chunk
		if i+chunk > len(*pkgs) {
			end = len(*pkgs)
		}
		result = r.db.Debug().Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "checksum"}},
			DoNothing: true,
		}).Create((*pkgs)[i:end])

		if result.Error != nil {
			return count, result.Error
		}
		count += result.RowsAffected
	}
	return count, result.Error
}

// existingChecksums passing in a list of checksums, returns the list of checksums
// that exist in the db
func (r rpmDaoImpl) existingChecksums(checksums []string) (*gorm.DB, []string) {
	var found []string
	result := r.db.Where("checksum in ?", checksums).Model(&models.Rpm{}).Pluck("checksum", &found)
	return result, found
}

func (r rpmDaoImpl) fetchRepo(uuid string) (models.Repository, error) {
	found := models.Repository{}
	result := r.db.
		Where("UUID = ?", uuid).
		First(&found)

	if result.Error != nil {
		return found, result.Error
	}
	return found, nil
}

// InsertForRepository inserts a set of yum packages for a given repository
//   and removes any that are not in the list.  This will involve inserting the RPMs
//   if not present, and adding or removing any associations to the Repository
//   Returns a count of new RPMs added to the system (not the repo), as well as any error
func (r rpmDaoImpl) InsertForRepository(repoUuid string, pkgs []yum.Package) (int64, error) {
	var rowsAffected int64

	repo, err := r.fetchRepo(repoUuid)
	if err != nil {
		return rowsAffected, err
	}
	checksums := make([]string, len(pkgs))
	for i := 0; i < len(pkgs); i++ {
		checksums[i] = pkgs[i].Checksum.Value
	}

	result, existingChecksums := r.existingChecksums(checksums)
	if result.Error != nil {
		return rowsAffected, result.Error
	}

	dbPkgs := FilteredConvert(pkgs, existingChecksums)

	rowsAffected, error := r.PagedRpmInsert(&dbPkgs)
	if error != nil {
		return rowsAffected, result.Error
	}

	var rpmUuids []string
	//Now fetch the uuids of all the rpms we want associated to the repository
	result = r.db.Where("checksum in ?", checksums).Model(&models.Rpm{}).Pluck("uuid", &rpmUuids)
	if result.Error != nil {
		return rowsAffected, result.Error
	}

	//Delete RepositoryRpm entries we don't need
	error = r.deleteUnneeded(repo, rpmUuids)
	if error != nil {
		return rowsAffected, result.Error
	}

	//Add the RepositoryRpm entries we do need
	associations := prepRepositoryRpms(repo, rpmUuids)
	result = r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "repository_uuid"}, {Name: "rpm_uuid"}},
		DoNothing: true}).Create(&associations)

	return rowsAffected, result.Error
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
func (r rpmDaoImpl) deleteUnneeded(repo models.Repository, rpm_uuids []string) error {
	//First get uuids that are there:
	var existing_rpm_uuids []string

	r.db.Model(&models.RepositoryRpm{}).Where("repository_uuid = ?", repo.UUID).Pluck("rpm_uuid", &existing_rpm_uuids)

	rpmsToDelete := difference(existing_rpm_uuids, rpm_uuids)

	result := r.db.Where("repositories_rpms.repository_uuid = ? and repositories_rpms.rpm_uuid in ?", repo.UUID, rpmsToDelete).Delete(&models.RepositoryRpm{})
	return result.Error
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
//	while filtering out any checksums that are in the excludedChecksums parameter
func FilteredConvert(yumPkgs []yum.Package, excludeChecksums []string) []models.Rpm {
	var dbPkgs []models.Rpm
	for i := 0; i < len(yumPkgs); i++ {
		yumPkg := yumPkgs[i]
		if !stringInSlice(yumPkg.Checksum.Value, excludeChecksums) {
			epoch := yumPkg.Version.Epoch
			dbPkgs = append(dbPkgs, models.Rpm{
				Base: models.Base{
					UUID: uuid.NewString(),
				},
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
