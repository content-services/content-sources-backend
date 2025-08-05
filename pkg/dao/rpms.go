package dao

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/roadmap_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/utils"
	"github.com/content-services/tang/pkg/tangy"
	"github.com/content-services/yummy/pkg/yum"
	"github.com/lib/pq"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var DbInClauseLimit = 60000

type rpmDaoImpl struct {
	db            *gorm.DB
	roadmapClient roadmap_client.RoadmapClient
}

func GetRpmDao(db *gorm.DB, roadmapClient roadmap_client.RoadmapClient) RpmDao {
	// Return DAO instance
	return &rpmDaoImpl{
		db:            db,
		roadmapClient: roadmapClient,
	}
}

func (r *rpmDaoImpl) List(
	ctx context.Context,
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
				RepositoryDBErrorToApi(err, &repositoryConfigUUID)
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

	if err := r.db.WithContext(ctx).
		Preload("Repository").
		Find(&repositoryConfig, "uuid = ?", repositoryConfigUUID).
		Error; err != nil {
		return api.RepositoryRpmCollectionResponse{}, totalRpms, err
	}

	filteredDB := r.db.WithContext(ctx).Model(&repoRpms).Joins(strings.Join([]string{"inner join", models.TableNameRpmsRepositories, "on uuid = rpm_uuid"}, " ")).
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

func popularRepoUrls() []string {
	var urls []string
	for _, repo := range config.PopularRepos {
		urls = append(urls, repo.URL)
	}
	return urls
}

func (r *rpmDaoImpl) Search(ctx context.Context, orgID string, request api.ContentUnitSearchRequest) ([]api.SearchRpmResponse, error) {
	// Retrieve the repository id list
	if orgID == "" {
		return nil, fmt.Errorf("orgID cannot be an empty string")
	}
	// Verify length of URLs or UUIDs is greater than 1
	if err := checkRequestUrlAndUuids(request); err != nil {
		return nil, err
	}
	// Set to default request limit if null or request limit max (500) if greater than max
	request = checkRequestLimit(request)

	uuids := request.UUIDs

	// Handle whitespaces and slashes in URLs
	var urls []string
	for _, url := range request.URLs {
		url = models.CleanupURL(url)
		urls = append(urls, url)
	}

	// Check that repository uuids and urls exist
	uuidsValid, urlsValid, uuid, url := checkForValidRepoUuidsUrls(ctx, uuids, urls, r.db)
	if !uuidsValid {
		return []api.SearchRpmResponse{}, &ce.DaoError{
			NotFound: true,
			Message:  "Could not find repository with UUID: " + uuid,
		}
	}
	if !urlsValid {
		return []api.SearchRpmResponse{}, &ce.DaoError{
			NotFound: true,
			Message:  "Could not find repository with URL: " + url,
		}
	}

	// Lookup repo uuids to search
	var repoUUIDs []string
	readableRepos := readableRepositoryQuery(r.db.WithContext(ctx), orgID, urls, uuids).Pluck("repositories.uuid", &repoUUIDs)

	// https://github.com/go-gorm/gorm/issues/5318
	dataResponse := []api.SearchRpmResponse{}
	db := r.db.WithContext(ctx).
		Select("DISTINCT ON(rpms.name) rpms.name as package_name", "rpms.summary", "rpms.uuid").
		Table(models.TableNameRpm).
		Joins("inner join repositories_rpms on repositories_rpms.rpm_uuid = rpms.uuid").
		Where("repositories_rpms.repository_uuid in (?)", readableRepos)

	if len(request.ExactNames) != 0 {
		db = db.Where("rpms.name in (?)", request.ExactNames)
	} else {
		db = db.Where("rpms.name ILIKE ?", fmt.Sprintf("%s%%", request.Search))
	}

	db = db.Order("rpms.name ASC").
		Limit(*request.Limit).
		Scan(&dataResponse)

	if db.Error != nil {
		return nil, db.Error
	}

	// Add module info to the response if requested
	if request.IncludePackageSources && len(dataResponse) > 0 {
		err := r.addPackageSources(ctx, dataResponse, repoUUIDs)
		if err != nil {
			return nil, err
		}
	}

	return dataResponse, nil
}

func (r *rpmDaoImpl) addPackageSources(ctx context.Context, rpmResponse []api.SearchRpmResponse, repoUUIDs []string) error {
	var moduleInfo []models.ModuleStream

	pkgNames := make([]string, len(rpmResponse))
	for i, rpm := range rpmResponse {
		pkgNames[i] = rpm.PackageName
	}

	err := r.db.WithContext(ctx).
		Table("module_streams").
		Joins("inner join repositories_module_streams rms ON rms.module_stream_uuid = module_streams.uuid").
		Select("DISTINCT ON (name, stream) name, stream, context, arch, version, description, package_names").
		Where("rms.repository_uuid in (?)", repoUUIDs).
		Where("module_streams.package_names && ?", clause.Expr{SQL: "ARRAY[?]", Vars: []interface{}{pkgNames}, WithoutParentheses: true}).
		Order("module_streams.name, module_streams.stream, module_streams.version desc").
		Find(&moduleInfo).Error
	if err != nil {
		return err
	}

	var unmatchedRPMs []string
	for i, rpm := range rpmResponse {
		var packageSources []api.PackageSourcesResponse
		var pkgStreamFound bool
		var moduleStreamFound bool

		for _, m := range moduleInfo {
			if utils.Contains(m.PackageNames, rpm.PackageName) {
				module := api.PackageSourcesResponse{
					Type:        "module",
					Name:        m.Name,
					Stream:      m.Stream,
					Context:     m.Context,
					Arch:        m.Arch,
					Version:     m.Version,
					Description: m.Description,
				}
				packageSources = append(packageSources, module)
			}
		}

		packageSources, pkgStreamFound, err = r.addRoadmapPackageStreams(ctx, rpm, packageSources)
		if err != nil {
			return err
		}

		packageSources, moduleStreamFound, err = r.addRoadmapModuleStreams(ctx, packageSources)
		if err != nil {
			return err
		}

		if !pkgStreamFound && !moduleStreamFound {
			unmatchedRPMs = append(unmatchedRPMs, rpm.PackageName)
		}

		rpmResponse[i].PackageSources = packageSources
	}

	// If any RPMs not found in roadmap API, get lifecycle information of highest matching RHEL version
	// and add RHEL lifecycle information as package source
	if len(unmatchedRPMs) > 0 {
		packageSourcesMap, err := r.addRoadmapRhelEol(ctx, unmatchedRPMs, repoUUIDs)
		if err != nil {
			return err
		}

		for i, rpm := range rpmResponse {
			if packageSource, found := packageSourcesMap[rpm.PackageName]; found {
				rpmResponse[i].PackageSources = append(rpmResponse[i].PackageSources, packageSource)
			}
		}
	}
	return nil
}

// addRoadmapPackageStreams calls the roadmap API to add the package streams associated to the given RPM to packageSources. Also adds
// the lifecycle start_date and end_date
func (r *rpmDaoImpl) addRoadmapPackageStreams(ctx context.Context, rpm api.SearchRpmResponse, packageSources []api.PackageSourcesResponse) ([]api.PackageSourcesResponse, bool, error) {
	if !config.RoadmapConfigured() {
		return packageSources, false, nil
	}

	appstreams, _, err := r.roadmapClient.GetAppstreams(ctx)
	if err != nil {
		return packageSources, false, err
	}

	var streamFound bool
	packageSourcesUpdated := packageSources
	for _, appstreamEntity := range appstreams.Data {
		if appstreamEntity.Name == rpm.PackageName && appstreamEntity.Impl == "package" {
			packageStream := api.PackageSourcesResponse{
				Type:      "package",
				Name:      rpm.PackageName,
				Stream:    appstreamEntity.Stream,
				StartDate: appstreamEntity.StartDate,
				EndDate:   appstreamEntity.EndDate,
			}
			packageSourcesUpdated = append(packageSourcesUpdated, packageStream)
			streamFound = true
		}
	}

	return packageSourcesUpdated, streamFound, nil
}

// addRoadmapModuleStreams calls the roadmap API to add the module stream lifecycle information to the module
// streams in packageSources
func (r *rpmDaoImpl) addRoadmapModuleStreams(ctx context.Context, packageSources []api.PackageSourcesResponse) ([]api.PackageSourcesResponse, bool, error) {
	if !config.RoadmapConfigured() {
		return packageSources, false, nil
	}

	appstreams, _, err := r.roadmapClient.GetAppstreams(ctx)
	if err != nil {
		return packageSources, false, err
	}

	var streamFound bool
	packageSourcesUpdated := packageSources
	for _, appstreamEntity := range appstreams.Data {
		for j, source := range packageSourcesUpdated {
			if appstreamEntity.Name == source.Name &&
				appstreamEntity.Stream == source.Stream &&
				appstreamEntity.Impl == "dnf_module" {
				packageSourcesUpdated[j].StartDate = appstreamEntity.StartDate
				packageSourcesUpdated[j].EndDate = appstreamEntity.EndDate
				streamFound = true
			}
		}
	}
	return packageSourcesUpdated, streamFound, nil
}

func (r *rpmDaoImpl) addRoadmapRhelEol(ctx context.Context, unmatchedRPMs []string, repoUUIDs []string) (map[string]api.PackageSourcesResponse, error) {
	type queryRes struct {
		Name     string
		Versions pq.StringArray `gorm:"type:text[]"`
	}
	var res []queryRes
	err := r.db.Debug().WithContext(ctx).Table(models.TableNameRpm).
		Select("rpms.name, repository_configurations.versions").
		Joins("INNER JOIN repositories_rpms ON rpms.uuid = repositories_rpms.rpm_uuid").
		Joins("INNER JOIN repository_configurations ON repository_configurations.repository_uuid = repositories_rpms.repository_uuid").
		Where("repository_configurations.org_id = ? AND rpms.name in ? AND repository_configurations.repository_uuid in ?", config.RedHatOrg, unmatchedRPMs, repoUUIDs).
		Find(&res).Error

	if err != nil {
		return nil, err
	}

	packageMap := make(map[string]int)
	for _, pkg := range res {
		if len(pkg.Versions) == 0 {
			continue
		}
		versionInt, err := strconv.Atoi(pkg.Versions[0])
		if err != nil {
			return nil, err
		}
		if existingVersion, found := packageMap[pkg.Name]; found {
			if versionInt > existingVersion {
				packageMap[pkg.Name] = versionInt
			}
		} else {
			packageMap[pkg.Name] = versionInt
		}
	}

	rhelEolMap, err := r.roadmapClient.GetRhelLifecycleForLatestMajorVersions(ctx)
	if err != nil {
		return nil, err
	}

	packageSourcesMap := make(map[string]api.PackageSourcesResponse)
	for name, version := range packageMap {
		packageSource := api.PackageSourcesResponse{
			Type:    "package",
			Name:    name,
			EndDate: rhelEolMap[version].EndDate,
		}
		packageSourcesMap[name] = packageSource
	}
	return packageSourcesMap, nil
}

func readableRepositoryQuery(dbWithContext *gorm.DB, orgID string, urls []string, uuids []string) *gorm.DB {
	orGroupPublicPrivatePopularShared := dbWithContext.
		Where("repository_configurations.org_id IN (?, ?, ?)", orgID, config.RedHatOrg, config.CommunityOrg).
		Or("repositories.public").
		Or("repositories.url in ?", popularRepoUrls())
	readableRepos := dbWithContext.Model(&models.Repository{}).
		Joins("left join repository_configurations on repositories.uuid = repository_configurations.repository_uuid").
		Where(orGroupPublicPrivatePopularShared).
		Where(dbWithContext.Where("repositories.url in ?", urls).
			Or("repository_configurations.uuid in ?", UuidifyStrings(uuids)))
	return readableRepos.Select("repositories.uuid")
}

func (r *rpmDaoImpl) fetchRepo(ctx context.Context, uuid string) (models.Repository, error) {
	found := models.Repository{}
	if err := r.db.WithContext(ctx).
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
func (r *rpmDaoImpl) InsertForRepository(ctx context.Context, repoUuid string, pkgs []yum.Package) (int64, error) {
	var (
		err               error
		repo              models.Repository
		existingChecksums []string
	)

	// Retrieve Repository record
	if repo, err = r.fetchRepo(ctx, repoUuid); err != nil {
		return 0, fmt.Errorf("failed to fetchRepo: %w", err)
	}

	// Build the list of checksums from the provided packages
	checksums := make([]string, len(pkgs))
	for i := 0; i < len(pkgs); i++ {
		checksums[i] = pkgs[i].Checksum.Value
	}

	// Given the list of checksums, retrieve the list of the ones that exists
	// in the 'rpm' table (whatever is the repository that it could belong)
	// Use batches to work under the postgres limit
	for i := 0; i < len(checksums); i = i + DbInClauseLimit {
		batchChecksums := []string{}
		final := i + DbInClauseLimit
		if final > len(checksums)-1 {
			final = len(checksums)
		}
		err := r.db.WithContext(ctx).
			Where("checksum in (?)", checksums[i:final]).
			Model(&models.Rpm{}).
			Pluck("checksum", &batchChecksums).Error

		if err != nil {
			return 0, fmt.Errorf("failed retrieving existing checksum in rpms: %w", err)
		}
		existingChecksums = append(existingChecksums, batchChecksums...)
	}

	// Given a slice of yum.Package, it filters the ones which checksum exists
	// in existingChecksums and return a slice of models.Rpm
	dbPkgs := FilteredConvert(pkgs, existingChecksums)

	// Insert the filtered packages in rpms table
	result := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "checksum"}},
		DoNothing: true,
	}).Create(dbPkgs)
	if result.Error != nil {
		return 0, fmt.Errorf("failed to PagedRpmInsert: %w", err)
	}

	// Now fetch the uuids of all the rpms we want associated to the repository
	var rpmUuids []string
	for i := 0; i < len(checksums); i = i + DbInClauseLimit {
		batchUuids := []string{}

		final := i + DbInClauseLimit
		if final > len(checksums)-1 {
			final = len(checksums)
		}
		if err = r.db.WithContext(ctx).
			Where("checksum in (?)", checksums[i:final]).
			Model(&models.Rpm{}).
			Pluck("uuid", &batchUuids).Error; err != nil {
			return 0, fmt.Errorf("failed retrieving rpms.uuid for the package checksums: %w", err)
		}
		rpmUuids = append(rpmUuids, batchUuids...)
	}

	// Delete Rpm and RepositoryRpm entries we don't need
	if err = r.deleteUnneeded(ctx, repo, rpmUuids); err != nil {
		return 0, fmt.Errorf("failed to deleteUnneeded: %w", err)
	}

	// Add the RepositoryRpm entries we do need
	associations := prepRepositoryRpms(repo, rpmUuids)
	result = r.db.WithContext(ctx).Clauses(clause.OnConflict{
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
func (r *rpmDaoImpl) deleteUnneeded(ctx context.Context, repo models.Repository, rpm_uuids []string) error {
	// First get uuids that are there:
	var (
		existing_rpm_uuids []string
	)

	// Read existing rpm_uuid associated to repository_uuid
	if err := r.db.WithContext(ctx).Model(&models.RepositoryRpm{}).
		Where("repository_uuid = ?", repo.UUID).
		Pluck("rpm_uuid", &existing_rpm_uuids).
		Error; err != nil {
		return err
	}

	rpmsToDelete := difference(existing_rpm_uuids, rpm_uuids)

	// Delete the many2many relationship for the unneeded rpms
	if err := r.db.WithContext(ctx).
		Unscoped().
		Where("repositories_rpms.repository_uuid = ?", repo.UUID).
		Where("repositories_rpms.rpm_uuid in (?)", rpmsToDelete).
		Delete(&models.RepositoryRpm{}).
		Error; err != nil {
		return err
	}

	return nil
}

func (r *rpmDaoImpl) OrphanCleanup(ctx context.Context) error {
	var danglingRpmUuids []string

	// Retrieve dangling rpms.uuid
	if err := r.db.WithContext(ctx).
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
	if err := r.db.WithContext(ctx).
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

	// Check that snapshot uuids exist
	uuids := request.UUIDs
	uuidsValid, uuid := checkForValidSnapshotUuids(ctx, uuids, r.db)
	if !uuidsValid {
		return []api.SearchRpmResponse{}, &ce.DaoError{
			NotFound: true,
			Message:  "Could not find snapshot with UUID: " + uuid,
		}
	}

	pulpHrefs := []string{}
	res := readableSnapshots(r.db.WithContext(ctx), orgId).Where("snapshots.UUID in ?", UuidifyStrings(request.UUIDs)).Pluck("version_href", &pulpHrefs)
	if res.Error != nil {
		return response, fmt.Errorf("failed to query the db for snapshots: %w", res.Error)
	}
	if config.Tang == nil {
		return response, fmt.Errorf("no tang configuration present")
	}

	if len(pulpHrefs) == 0 {
		return response, nil
	}

	pkgs, err := (*config.Tang).RpmRepositoryVersionPackageSearch(ctx, pulpHrefs, request.Search, *request.Limit)
	if err != nil {
		return response, fmt.Errorf("error querying packages in snapshots: %w", err)
	}
	for _, pkg := range pkgs {
		response = append(response, api.SearchRpmResponse{
			PackageName: pkg.Name,
			Summary:     pkg.Summary,
		})
	}

	// Add package sources to the response if requested
	if request.IncludePackageSources && len(response) > 0 {
		var repoUUIDs []string
		err = r.fetchRepoUUIDsForSnapshots(ctx, orgId, request.UUIDs, &repoUUIDs)
		if err != nil {
			return nil, fmt.Errorf("failed to query the db for repo uuids from snapshots: %w", res.Error)
		}
		err = r.addPackageSources(ctx, response, repoUUIDs)
		if err != nil {
			return nil, err
		}
	}

	return response, nil
}

func (r *rpmDaoImpl) fetchRepoUUIDsForSnapshots(ctx context.Context, orgId string, snapUUIDs []string, repoUUIDs *[]string) error {
	err := r.db.WithContext(ctx).
		Table("repository_configurations").
		Select("repository_uuid").
		Where("uuid in (?)",
			readableSnapshots(r.db.WithContext(ctx), orgId).
				Where("snapshots.uuid in ?", UuidifyStrings(snapUUIDs)).
				Select("repository_configuration_uuid"),
		).
		Pluck("repository_uuid", &repoUUIDs).Error

	return err
}

func (r *rpmDaoImpl) ListSnapshotRpms(ctx context.Context, orgId string, snapshotUUIDs []string, search string, pageOpts api.PaginationData) ([]api.SnapshotRpm, int, error) {
	response := []api.SnapshotRpm{}

	// Check that snapshot uuids exist
	uuidsValid, uuid := checkForValidSnapshotUuids(ctx, snapshotUUIDs, r.db)
	if !uuidsValid {
		return []api.SnapshotRpm{}, 0, &ce.DaoError{
			NotFound: true,
			Message:  "Could not find snapshot with UUID: " + uuid,
		}
	}

	pulpHrefs := []string{}
	res := readableSnapshots(r.db.WithContext(ctx), orgId).Where("snapshots.UUID in ?", UuidifyStrings(snapshotUUIDs)).Pluck("version_href", &pulpHrefs)
	if res.Error != nil {
		return response, 0, fmt.Errorf("failed to query the db for snapshots: %w", res.Error)
	}
	if config.Tang == nil {
		return response, 0, fmt.Errorf("no tang configuration present")
	}

	if len(pulpHrefs) == 0 {
		return response, 0, nil
	}

	pkgs, total, err := (*config.Tang).RpmRepositoryVersionPackageList(ctx, pulpHrefs, tangy.RpmListFilters{Name: search}, tangy.PageOptions{
		Offset: pageOpts.Offset,
		Limit:  pageOpts.Limit,
	})

	if err != nil {
		return response, 0, fmt.Errorf("error querying packages in snapshots: %w", err)
	}
	for _, pkg := range pkgs {
		response = append(response, api.SnapshotRpm{
			Name:    pkg.Name,
			Arch:    pkg.Arch,
			Version: pkg.Version,
			Release: pkg.Release,
			Epoch:   pkg.Epoch,
			Summary: pkg.Summary,
		})
	}
	return response, total, nil
}

func (r *rpmDaoImpl) DetectRpms(ctx context.Context, orgID string, request api.DetectRpmsRequest) (*api.DetectRpmsResponse, error) {
	if orgID == "" {
		return nil, fmt.Errorf("orgID cannot be an empty string")
	}
	// verify length of URLs or UUIDs is greater than 1
	if len(request.URLs) == 0 && len(request.UUIDs) == 0 {
		return nil, &ce.DaoError{
			BadValidation: true,
			Message:       "must contain at least 1 URL or 1 UUID",
		}
	}
	// set limit if not already and reject request if more than max requested
	if request.Limit == nil {
		request.Limit = utils.Ptr(api.ContentUnitSearchRequestLimitDefault)
	}
	if *request.Limit > api.ContentUnitSearchRequestLimitMaximum {
		return nil, &ce.DaoError{
			BadValidation: true,
			Message:       "Limit cannot be more than 500",
		}
	}

	uuids := request.UUIDs
	var missingRpms []string
	var dataResponse *api.DetectRpmsResponse
	var foundRpmsModel []string

	// handle whitespaces and slashes in URLs
	var urls []string
	for _, url := range request.URLs {
		url = models.CleanupURL(url)
		urls = append(urls, url)
	}

	// check that repository uuids and urls exist
	uuidsValid, urlsValid, uuid, url := checkForValidRepoUuidsUrls(ctx, uuids, urls, r.db)
	if !uuidsValid {
		return dataResponse, &ce.DaoError{
			NotFound: true,
			Message:  "Could not find repository with UUID: " + uuid,
		}
	}
	if !urlsValid {
		return dataResponse, &ce.DaoError{
			NotFound: true,
			Message:  "Could not find repository with URL: " + url,
		}
	}

	// find rpms associated with the repositories that match given rpm names
	orGroupPublicOrPrivate := r.db.Where("repository_configurations.org_id = ?", orgID).Or("repositories.public")
	db := r.db.WithContext(ctx).
		Select("DISTINCT ON(rpms.name) rpms.name AS found").
		Table(models.TableNameRpm).
		Joins("INNER JOIN repositories_rpms ON repositories_rpms.rpm_uuid = rpms.uuid").
		Joins("INNER JOIN repositories ON repositories.uuid = repositories_rpms.repository_uuid").
		Joins("LEFT JOIN repository_configurations ON repository_configurations.repository_uuid = repositories.uuid").
		Where(orGroupPublicOrPrivate).
		Where("rpms.name IN ?", request.RpmNames).
		Where(r.db.Where("repositories.url IN ?", urls).
			Or("repository_configurations.uuid IN ?", UuidifyStrings(uuids))).
		Where("repository_configurations.deleted_at is NULL").
		Order("rpms.name").
		Limit(*request.Limit).
		Scan(&foundRpmsModel)

	if db.Error != nil {
		return nil, db.Error
	}

	// convert model to response
	dataResponse = &api.DetectRpmsResponse{Found: []string{}}
	dataResponse.Found = foundRpmsModel

	// retrieve missing rpms by comparing requested rpms to the found rpms
	for _, requestedRpm := range request.RpmNames {
		if !stringInSlice(requestedRpm, dataResponse.Found) {
			if len(missingRpms) < *request.Limit {
				missingRpms = append(missingRpms, requestedRpm)
			}
		}
	}
	dataResponse.Missing = missingRpms

	// ensure there are no null values
	if dataResponse.Found == nil {
		dataResponse.Found = []string{}
	}
	if dataResponse.Missing == nil {
		dataResponse.Missing = []string{}
	}

	return dataResponse, nil
}

func (r *rpmDaoImpl) ListSnapshotErrata(ctx context.Context, orgId string, snapshotUUIDs []string, filters tangy.ErrataListFilters, pageOpts api.PaginationData) ([]api.SnapshotErrata, int, error) {
	response := []api.SnapshotErrata{}

	// Check that snapshot uuids exist
	uuidsValid, uuid := checkForValidSnapshotUuids(ctx, snapshotUUIDs, r.db)
	if !uuidsValid {
		return []api.SnapshotErrata{}, 0, &ce.DaoError{
			NotFound: true,
			Message:  "Could not find snapshot with UUID: " + uuid,
		}
	}

	pulpHrefs := []string{}
	res := readableSnapshots(r.db.WithContext(ctx), orgId).Where("snapshots.UUID in ?", UuidifyStrings(snapshotUUIDs)).Pluck("version_href", &pulpHrefs)

	if res.Error != nil {
		return response, 0, fmt.Errorf("failed to query the db for snapshots: %w", res.Error)
	}
	if config.Tang == nil {
		return response, 0, fmt.Errorf("no tang configuration present")
	}

	if len(pulpHrefs) == 0 {
		return response, 0, nil
	}

	pkgs, total, err := (*config.Tang).RpmRepositoryVersionErrataList(ctx, pulpHrefs, filters, tangy.PageOptions{
		Offset: pageOpts.Offset,
		Limit:  pageOpts.Limit,
		SortBy: pageOpts.SortBy,
	})

	if err != nil {
		return response, 0, fmt.Errorf("error querying packages in snapshots: %w", err)
	}

	for _, pkg := range pkgs {
		issuedDate := ""
		updatedDate := ""
		CVEs := []string{}
		if pkg.UpdatedDate != nil {
			if t, err := time.Parse(time.DateTime, *pkg.UpdatedDate); err == nil {
				updatedDate = t.UTC().Format(time.RFC3339)
			}
		}
		if t, err := time.Parse(time.DateTime, pkg.IssuedDate); err == nil {
			issuedDate = t.UTC().Format(time.RFC3339)
		}

		if pkg.CVEs != nil {
			CVEs = pkg.CVEs
		}
		response = append(response, api.SnapshotErrata{
			Id:              pkg.Id,
			ErrataId:        pkg.ErrataId,
			Title:           pkg.Title,
			Summary:         pkg.Summary,
			Description:     pkg.Description,
			IssuedDate:      issuedDate,
			UpdateDate:      updatedDate,
			Type:            pkg.Type,
			Severity:        pkg.Severity,
			RebootSuggested: pkg.RebootSuggested,
			CVEs:            CVEs,
		})
	}

	return response, total, nil
}

func (r *rpmDaoImpl) ListTemplateRpms(ctx context.Context, orgId string, templateUUID string, search string, pageOpts api.PaginationData) ([]api.SnapshotRpm, int, error) {
	response := []api.SnapshotRpm{}
	pulpHrefs := []string{}

	snapshots, err := r.fetchSnapshotsForTemplate(ctx, orgId, templateUUID)
	if err != nil {
		return response, 0, err
	}

	for _, snapshot := range snapshots {
		pulpHrefs = append(pulpHrefs, snapshot.VersionHref)
	}

	if config.Tang == nil {
		return response, 0, fmt.Errorf("no tang configuration present")
	}

	if len(pulpHrefs) == 0 {
		return response, 0, nil
	}

	pkgs, total, err := (*config.Tang).RpmRepositoryVersionPackageList(ctx, pulpHrefs, tangy.RpmListFilters{Name: search}, tangy.PageOptions{
		Offset: pageOpts.Offset,
		Limit:  pageOpts.Limit,
	})

	if err != nil {
		return response, 0, fmt.Errorf("error querying packages in templates: %w", err)
	}
	for _, pkg := range pkgs {
		response = append(response, api.SnapshotRpm{
			Name:    pkg.Name,
			Arch:    pkg.Arch,
			Version: pkg.Version,
			Release: pkg.Release,
			Epoch:   pkg.Epoch,
			Summary: pkg.Summary,
		})
	}
	return response, total, nil
}

func (r *rpmDaoImpl) fetchSnapshotsForTemplate(ctx context.Context, orgId string, templateUUID string) ([]models.Snapshot, error) {
	repoUuids := []string{}
	var template models.Template

	err := r.db.WithContext(ctx).
		Where("uuid = ? AND org_id = ?", UuidifyString(templateUUID), orgId).
		Preload("TemplateRepositoryConfigurations").First(&template).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return []models.Snapshot{}, &ce.DaoError{NotFound: true, Message: "Could not find template with UUID " + templateUUID}
		}
		return []models.Snapshot{}, err
	}

	for _, tRepoConfig := range template.TemplateRepositoryConfigurations {
		repoUuids = append(repoUuids, tRepoConfig.RepositoryConfigurationUUID)
	}

	var templateDate time.Time
	if template.UseLatest {
		templateDate = time.Now()
	} else {
		templateDate = template.Date
	}

	snapshots, err := GetSnapshotDao(r.db).FetchSnapshotsModelByDateAndRepository(ctx, orgId, api.ListSnapshotByDateRequest{RepositoryUUIDS: repoUuids, Date: templateDate})
	if err != nil {
		return []models.Snapshot{}, err
	}

	return snapshots, nil
}

func (r *rpmDaoImpl) ListTemplateErrata(ctx context.Context, orgId string, templateUUID string, filters tangy.ErrataListFilters, pageOpts api.PaginationData) ([]api.SnapshotErrata, int, error) {
	response := []api.SnapshotErrata{}
	pulpHrefs := []string{}

	snapshots, err := r.fetchSnapshotsForTemplate(ctx, orgId, templateUUID)
	if err != nil {
		return response, 0, err
	}

	for _, snapshot := range snapshots {
		pulpHrefs = append(pulpHrefs, snapshot.VersionHref)
	}

	if config.Tang == nil {
		return response, 0, fmt.Errorf("no tang configuration present")
	}

	if len(pulpHrefs) == 0 {
		return response, 0, nil
	}

	pkgs, total, err := (*config.Tang).RpmRepositoryVersionErrataList(ctx, pulpHrefs, filters, tangy.PageOptions{
		Offset: pageOpts.Offset,
		Limit:  pageOpts.Limit,
		SortBy: pageOpts.SortBy,
	})

	if err != nil {
		return response, 0, fmt.Errorf("error querying errata in snapshots: %w", err)
	}

	for _, pkg := range pkgs {
		issuedDate := ""
		updatedDate := ""
		CVEs := []string{}
		if pkg.UpdatedDate != nil {
			if t, err := time.Parse(time.DateTime, *pkg.UpdatedDate); err == nil {
				updatedDate = t.UTC().Format(time.RFC3339)
			}
		}
		if t, err := time.Parse(time.DateTime, pkg.IssuedDate); err == nil {
			issuedDate = t.UTC().Format(time.RFC3339)
		}

		if pkg.CVEs != nil {
			CVEs = pkg.CVEs
		}
		response = append(response, api.SnapshotErrata{
			Id:              pkg.Id,
			ErrataId:        pkg.ErrataId,
			Title:           pkg.Title,
			Summary:         pkg.Summary,
			Description:     pkg.Description,
			IssuedDate:      issuedDate,
			UpdateDate:      updatedDate,
			Type:            pkg.Type,
			Severity:        pkg.Severity,
			RebootSuggested: pkg.RebootSuggested,
			CVEs:            CVEs,
		})
	}

	return response, total, nil
}
