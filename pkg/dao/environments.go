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

type environmentDaoImpl struct {
	db *gorm.DB
}

func GetEnvironmentDao(db *gorm.DB) EnvironmentDao {
	// Return DAO instance
	return environmentDaoImpl{
		db: db,
	}
}

func (r environmentDaoImpl) List(orgID string, repositoryConfigUUID string, limit int, offset int, search string, sortBy string) (api.RepositoryEnvironmentCollectionResponse, int64, error) {
	// Check arguments
	if orgID == "" {
		return api.RepositoryEnvironmentCollectionResponse{}, 0, fmt.Errorf("orgID can not be an empty string")
	}

	var totalEnvironments int64
	repoEnvironments := []models.Environment{}

	if ok, err := isOwnedRepository(r.db, orgID, repositoryConfigUUID); !ok {
		if err != nil {
			return api.RepositoryEnvironmentCollectionResponse{},
				totalEnvironments,
				DBErrorToApi(err)
		}
		return api.RepositoryEnvironmentCollectionResponse{},
			totalEnvironments,
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
		return api.RepositoryEnvironmentCollectionResponse{}, totalEnvironments, err
	}

	filteredDB := r.db.Model(&repoEnvironments).Joins(strings.Join([]string{"inner join", models.TableNameEnvironmentsRepositories, "on uuid = environment_uuid"}, " ")).
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
	}

	order := convertSortByToSQL(sortBy, sortMap, "name asc")

	filteredDB = filteredDB.
		Order(order).
		Count(&totalEnvironments).
		Offset(offset).
		Limit(limit).
		Find(&repoEnvironments)

	if filteredDB.Error != nil {
		return api.RepositoryEnvironmentCollectionResponse{}, totalEnvironments, filteredDB.Error
	}

	// Return the environment list
	repoEnvironmentResponse := r.RepositoryEnvironmentListFromModelToResponse(repoEnvironments)
	return api.RepositoryEnvironmentCollectionResponse{
		Data: repoEnvironmentResponse,
		Meta: api.ResponseMetadata{
			Count:  totalEnvironments,
			Offset: offset,
			Limit:  limit,
		},
	}, totalEnvironments, nil
}

func (r environmentDaoImpl) RepositoryEnvironmentListFromModelToResponse(repoEnvironment []models.Environment) []api.RepositoryEnvironment {
	repos := make([]api.RepositoryEnvironment, len(repoEnvironment))
	for i := 0; i < len(repoEnvironment); i++ {
		r.modelToApiFields(&repoEnvironment[i], &repos[i])
	}
	return repos
}

// apiFieldsToModel transform from database model to API request.
// in the source models.Environment structure.
// out the output api.RepositoryEnvironment structure.
//
// NOTE: This encapsulates transformation into environmentDaoImpl implementation
// as the methods are not used outside; if they were used
// out of this place, decouple into a new struct and make
// the methods public.
func (r environmentDaoImpl) modelToApiFields(in *models.Environment, out *api.RepositoryEnvironment) {
	if in == nil || out == nil {
		return
	}
	out.UUID = in.Base.UUID
	out.ID = in.ID
	out.Name = in.Name
	out.Description = in.Description
}

func (r environmentDaoImpl) Search(orgID string, request api.ContentUnitSearchRequest) ([]api.SearchEnvironmentResponse, error) {
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

	// This implements the following SELECT statement:
	//
	// SELECT DISTINCT ON (environment.name)
	//        environments.name, environments.description
	// FROM environments
	//      inner join repositories_environments on repositories_environments.environment_uuid = environments.uuid
	//      inner join repositories on repositories.uuid = repositories_environments.repository_uuid
	//      left join repository_configurations on repository_configurations.repository_uuid = repositories.uuid
	// WHERE (repository_configurations.org_id = 'acme' OR repositories.public)
	//       AND ( repositories.url in (...)
	//             OR repository_configurations.uuid in (...)
	//       )
	//       AND environments.name LIKE 'demo%'
	// ORDER BY environments.name DESC
	// LIMIT 20;

	// https://github.com/go-gorm/gorm/issues/5318
	dataResponse := []api.SearchEnvironmentResponse{}
	orGroupPublicOrPrivate := r.db.Where("repository_configurations.org_id = ?", orgID).Or("repositories.public")
	db := r.db.
		Select("DISTINCT ON(environments.name, environments.id) environments.name as environment_name", "environments.id", "environments.description").
		Table(models.TableNameEnvironment).
		Joins("inner join repositories_environments on repositories_environments.environment_uuid = environments.uuid").
		Joins("inner join repositories on repositories.uuid = repositories_environments.repository_uuid").
		Joins("left join repository_configurations on repository_configurations.repository_uuid = repositories.uuid").
		Where(orGroupPublicOrPrivate).
		Where("environments.name ILIKE ?", fmt.Sprintf("%%%s%%", request.Search)).
		Where(r.db.Where("repositories.url in ?", urls).
			Or("repository_configurations.uuid in ?", UuidifyStrings(uuids))).
		Order("environments.name ASC").
		Limit(*request.Limit).
		Scan(&dataResponse)

	if db.Error != nil {
		return nil, db.Error
	}

	return dataResponse, nil
}

func (r environmentDaoImpl) fetchRepo(uuid string) (models.Repository, error) {
	found := models.Repository{}
	if err := r.db.
		Where("UUID = ?", uuid).
		First(&found).
		Error; err != nil {
		return found, err
	}
	return found, nil
}

// InsertForRepository inserts a set of yum environments for a given repository
// and removes any that are not in the list.  This will involve inserting the environments
// if not present, and adding or removing any associations to the Repository
// Returns a count of new environments added to the system (not the repo), as well as any error
func (r environmentDaoImpl) InsertForRepository(repoUuid string, environments []yum.Environment) (int64, error) {
	var (
		err          error
		repo         models.Repository
		existingEnvs []string
	)

	// Retrieve Repository record
	if repo, err = r.fetchRepo(repoUuid); err != nil {
		return 0, fmt.Errorf("failed to fetchRepo: %w", err)
	}

	// Build list of ids and names to deduplicate on
	ids := make([]string, len(environments))
	names := make([]string, len(environments))
	for i := 0; i < len(environments); i++ {
		ids[i] = environments[i].ID
		names[i] = string(environments[i].Name)
	}

	// Given the list of ids and names, retrieve the list of the ones that exists
	// in the 'environments' table (whatever is the repository that it could belong)
	if err = r.db.
		Where("id in (?)", ids).
		Where("name in (?)", names).
		Model(&models.Environment{}).
		Pluck("id", &existingEnvs).Error; err != nil {
		return 0, fmt.Errorf("failed retrieving existing id in environments: %w", err)
	}

	// Given a slice of yum.Environment, it filters the envs
	// in existingEvs and return a slice of models.Environment
	dbEnvironments := FilteredConvertEnvironments(environments, existingEnvs)

	// Insert the filtered environments in environments table
	result := r.db.Create(dbEnvironments)
	if result.Error != nil {
		return 0, fmt.Errorf("failed to PagedEnvironmentInsert: %w", err)
	}

	// Now fetch the uuids of all the environments we want associated to the repository
	var environmentUuids []string
	if err = r.db.
		Where("id in (?)", ids).
		Where("name in (?)", names).
		Model(&models.Environment{}).
		Pluck("uuid", &environmentUuids).Error; err != nil {
		return 0, fmt.Errorf("failed retrieving environments.uuid for the given environments: %w", err)
	}

	// Delete Environment and RepositoryEnvironment entries we don't need
	if err = r.deleteUnneeded(repo, environmentUuids); err != nil {
		return 0, fmt.Errorf("failed to deleteUnneeded: %w", err)
	}

	// Add the RepositoryEnvironment entries we do need
	associations := prepRepositoryEnvironments(repo, environmentUuids)
	result = r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "repository_uuid"}, {Name: "environment_uuid"}},
		DoNothing: true}).
		Create(&associations)
	if result.Error != nil {
		return result.RowsAffected, fmt.Errorf("failed to Create: %w", result.Error)
	}

	return result.RowsAffected, err
}

// prepRepositoryEnvironments converts a list of environment_uuids to a list of RepositoryEnvironment Objects
func prepRepositoryEnvironments(repo models.Repository, environmentUuids []string) []models.RepositoryEnvironment {
	repoEnvironments := make([]models.RepositoryEnvironment, len(environmentUuids))
	for i := 0; i < len(environmentUuids); i++ {
		repoEnvironments[i].RepositoryUUID = repo.UUID
		repoEnvironments[i].EnvironmentUUID = environmentUuids[i]
	}
	return repoEnvironments
}

// deleteUnneeded removes any RepositoryEnvironment entries that are not in the list of environmentUuids
func (r environmentDaoImpl) deleteUnneeded(repo models.Repository, environmentUuids []string) error {
	// First get uuids that are there:
	var (
		existingEnvironmentUuids []string
	)

	// Read existing environment_uuid associated to repository_uuid
	if err := r.db.Model(&models.RepositoryEnvironment{}).
		Where("repository_uuid = ?", repo.UUID).
		Pluck("environment_uuid", &existingEnvironmentUuids).
		Error; err != nil {
		return err
	}

	environmentsToDelete := difference(existingEnvironmentUuids, environmentUuids)

	// Delete the many2many relationship for the unneeded environments
	if err := r.db.
		Unscoped().
		Where("repositories_environments.repository_uuid = ?", repo.UUID).
		Where("repositories_environments.environment_uuid in (?)", environmentsToDelete).
		Delete(&models.RepositoryEnvironment{}).
		Error; err != nil {
		return err
	}

	return nil
}

func (r environmentDaoImpl) OrphanCleanup() error {
	var danglingEnvironmentUuids []string

	// Retrieve dangling environments.uuid
	if err := r.db.
		Model(&models.Environment{}).
		Where("repositories_environments.environment_uuid is NULL").
		Joins("left join repositories_environments on environments.uuid = repositories_environments.environment_uuid").
		Pluck("environments.uuid", &danglingEnvironmentUuids).
		Error; err != nil {
		return err
	}

	if len(danglingEnvironmentUuids) == 0 {
		return nil
	}

	// Remove dangling environments
	if err := r.db.
		Where("environments.uuid in (?)", danglingEnvironmentUuids).
		Delete(&models.Environment{}).
		Error; err != nil {
		return err
	}
	return nil
}

func (r environmentDaoImpl) SearchSnapshotEnvironments(ctx context.Context, orgId string, request api.SnapshotSearchRpmRequest) ([]api.SearchEnvironmentResponse, error) {
	response := []api.SearchEnvironmentResponse{}

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

	pkgs, err := (*config.Tang).RpmRepositoryVersionEnvironmentSearch(ctx, pulpHrefs, request.Search, *request.Limit)
	if err != nil {
		return response, fmt.Errorf("error querying packages in snapshots %w", err)
	}
	for _, pkg := range pkgs {
		response = append(response, api.SearchEnvironmentResponse{
			EnvironmentName: pkg.Name,
			Description:     pkg.Description,
			ID:              pkg.ID,
		})
	}
	return response, nil
}

// FilteredConvertEnvironments, given a list of yum.Environment objects, converts them to model.Environment
// while filtering out any envs that are in the excludedEnvs parameter
func FilteredConvertEnvironments(yumEnvironments []yum.Environment, excludedEnvs []string) []models.Environment {
	var dbEnvironments []models.Environment
	for _, yumEnvironment := range yumEnvironments {
		if !stringInSlice(string(yumEnvironment.ID), excludedEnvs) {
			dbEnvironments = append(dbEnvironments, models.Environment{
				ID:          yumEnvironment.ID,
				Name:        string(yumEnvironment.Name),
				Description: string(yumEnvironment.Description),
			})
		}
	}
	return dbEnvironments
}
