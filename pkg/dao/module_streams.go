package dao

import (
	"context"
	"fmt"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/tang/pkg/tangy"
	"github.com/content-services/yummy/pkg/yum"
	"github.com/lib/pq"
	"golang.org/x/exp/slices"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func GetModuleStreamsDao(db *gorm.DB) ModuleStreamDao {
	// Return DAO instance
	return &moduleStreamsImpl{db: db}
}

type moduleStreamsImpl struct {
	db *gorm.DB
}

func (r *moduleStreamsImpl) SearchRepositoryModuleStreams(ctx context.Context, orgID string, request api.SearchModuleStreamsRequest) (resp []api.SearchModuleStreams, err error) {
	if orgID == "" {
		return resp, fmt.Errorf("orgID can not be an empty string")
	}
	dbWithCtx := r.db.WithContext(ctx)
	if request.RpmNames == nil {
		request.RpmNames = []string{}
	}
	if len(request.UUIDs) == 0 && len(request.URLs) == 0 {
		return resp, &ce.DaoError{
			BadValidation: true,
			Message:       "must contain at least 1 Repository UUID or URL",
		}
	}

	uuids := []string{}
	if request.UUIDs != nil {
		uuids = request.UUIDs
	}

	urls := []string{}
	for _, url := range request.URLs {
		url = models.CleanupURL(url)
		urls = append(urls, url)
	}

	uuidsValid, urlsValid, uuid, url := checkForValidRepoUuidsUrls(ctx, uuids, urls, r.db)
	if !uuidsValid {
		return resp, &ce.DaoError{
			NotFound: true,
			Message:  "Could not find repository with UUID: " + uuid,
		}
	}
	if !urlsValid {
		return resp, &ce.DaoError{
			NotFound: true,
			Message:  "Could not find repository with URL: " + url,
		}
	}

	streams := []models.ModuleStream{}

	newestStreams := dbWithCtx.Model(&models.ModuleStream{}).
		Select("DISTINCT ON (name, stream) uuid").
		Joins("inner join repositories_module_streams on module_streams.uuid = repositories_module_streams.module_stream_uuid").
		Where("repositories_module_streams.repository_uuid in (?)", readableRepositoryQuery(dbWithCtx, orgID, urls, uuids))

	if len(request.RpmNames) > 0 {
		// we are checking if two arrays have things in common, so we have to conver to pq array type
		newestStreams = newestStreams.Where("module_streams.package_names && ?", pq.Array(request.RpmNames))
	}
	if request.Search != "" {
		newestStreams = newestStreams.Where("module_streams.name ilike ?", fmt.Sprintf("%%%s%%", request.Search))
	}
	newestStreams = newestStreams.Order("name, stream, version DESC")

	order := convertSortByToSQL(request.SortBy, map[string]string{"name": "name"}, "name asc")
	result := dbWithCtx.Model(&models.ModuleStream{}).Where("uuid in (?)", newestStreams).Order(fmt.Sprintf("%v, stream", order)).Find(&streams)

	if result.Error != nil {
		return resp, result.Error
	}
	return ModuleStreamsToCollectionResponse(streams), nil
}

func ModuleStreamsToCollectionResponse(modules []models.ModuleStream) (response []api.SearchModuleStreams) {
	mapping := make(map[string][]api.Stream)
	for _, mod := range modules {
		mapping[mod.Name] = append(mapping[mod.Name], api.Stream{
			Name:        mod.Name,
			Stream:      mod.Stream,
			Context:     mod.Context,
			Arch:        mod.Arch,
			Version:     mod.Version,
			Description: mod.Description,
			Profiles:    mod.Profiles,
		})
	}

	for k, v := range mapping {
		response = append(response, api.SearchModuleStreams{
			ModuleName: k,
			Streams:    v,
		})
	}
	return response
}

func (r *moduleStreamsImpl) SearchSnapshotModuleStreams(ctx context.Context, orgID string, request api.SearchSnapshotModuleStreamsRequest) ([]api.SearchModuleStreams, error) {
	if orgID == "" {
		return []api.SearchModuleStreams{}, fmt.Errorf("orgID can not be an empty string")
	}

	if request.RpmNames == nil {
		request.RpmNames = []string{}
	}

	if len(request.UUIDs) == 0 {
		return []api.SearchModuleStreams{}, &ce.DaoError{
			BadValidation: true,
			Message:       "must contain at least 1 snapshot UUID",
		}
	}

	response := []api.SearchModuleStreams{}

	// Check that snapshot uuids exist
	uuidsValid, uuid := checkForValidSnapshotUuids(ctx, request.UUIDs, r.db)
	if !uuidsValid {
		return []api.SearchModuleStreams{}, &ce.DaoError{
			NotFound: true,
			Message:  "Could not find snapshot with UUID: " + uuid,
		}
	}

	pulpHrefs := []string{}
	res := readableSnapshots(r.db.WithContext(ctx), orgID).Where("snapshots.UUID in ?", UuidifyStrings(request.UUIDs)).Pluck("version_href", &pulpHrefs)
	if res.Error != nil {
		return []api.SearchModuleStreams{}, fmt.Errorf("failed to query the db for snapshots: %w", res.Error)
	}
	if config.Tang == nil {
		return []api.SearchModuleStreams{}, fmt.Errorf("no tang configuration present")
	}

	if len(pulpHrefs) == 0 {
		return []api.SearchModuleStreams{}, nil
	}

	pkgs, err := (*config.Tang).RpmRepositoryVersionModuleStreamsList(ctx, pulpHrefs,
		tangy.ModuleStreamListFilters{RpmNames: request.RpmNames, Search: request.Search}, request.SortBy)

	if err != nil {
		return []api.SearchModuleStreams{}, fmt.Errorf("error querying module streams in snapshots: %w", err)
	}

	mappedModuleStreams := map[string][]api.Stream{}

	for _, pkg := range pkgs {
		if mappedModuleStreams[pkg.Name] == nil {
			mappedModuleStreams[pkg.Name] = []api.Stream{}
		}
		mappedModuleStreams[pkg.Name] = append(mappedModuleStreams[pkg.Name], api.Stream{
			Name:        pkg.Name,
			Stream:      pkg.Stream,
			Context:     pkg.Context,
			Arch:        pkg.Arch,
			Version:     pkg.Version,
			Description: pkg.Description,
			Profiles:    pkg.Profiles,
		})
	}

	for key, moduleStream := range mappedModuleStreams {
		response = append(response, api.SearchModuleStreams{
			ModuleName: key,
			Streams:    moduleStream,
		})
	}

	return response, nil
}

func (r moduleStreamsImpl) fetchRepo(ctx context.Context, uuid string) (models.Repository, error) {
	found := models.Repository{}
	if err := r.db.WithContext(ctx).
		Where("UUID = ?", uuid).
		First(&found).
		Error; err != nil {
		return found, err
	}
	return found, nil
}

// Converts an rpm NVREA into just the name
func extractRpmName(nvrea string) string {
	// rubygem-bson-debugsource-0:4.3.0-2.module+el8.1.0+3656+f80bfa1d.x86_64
	split := strings.Split(nvrea, "-")
	if len(split) < 3 {
		return nvrea
	}
	split = split[0 : len(split)-2]
	return strings.Join(split, "-")
}

func ModuleMdToModuleStreams(moduleMds []yum.ModuleMD) (moduleStreams []models.ModuleStream) {
	for _, m := range moduleMds {
		mStream := models.ModuleStream{
			Name:         m.Data.Name,
			Stream:       m.Data.Stream,
			Version:      m.Data.Version,
			Context:      m.Data.Context,
			Arch:         m.Data.Arch,
			Summary:      m.Data.Summary,
			Description:  m.Data.Description,
			Profiles:     map[string][]string{},
			PackageNames: []string{},
			Packages:     m.Data.Artifacts.Rpms,
		}
		for _, p := range m.Data.Artifacts.Rpms {
			mStream.PackageNames = append(mStream.PackageNames, extractRpmName(p))
		}
		slices.Sort(mStream.PackageNames) // Sort the package names so the hash is consistent
		mStream.HashValue = generateHash(mStream.ToHashString())
		for pName, p := range m.Data.Profiles {
			mStream.Profiles[pName] = p.Rpms
		}

		moduleStreams = append(moduleStreams, mStream)
	}
	return moduleStreams
}

// InsertForRepository inserts a set of yum module streams for a given repository
// and removes any that are not in the list.  This will involve inserting the package groups
// if not present, and adding or removing any associations to the Repository
// Returns a count of new package groups added to the system (not the repo), as well as any error
func (r moduleStreamsImpl) InsertForRepository(ctx context.Context, repoUuid string, modules []yum.ModuleMD) (int64, error) {
	var (
		err  error
		repo models.Repository
	)
	ctxDb := r.db.WithContext(ctx)

	// Retrieve Repository record
	if repo, err = r.fetchRepo(ctx, repoUuid); err != nil {
		return 0, fmt.Errorf("failed to fetchRepo: %w", err)
	}

	moduleStreams := ModuleMdToModuleStreams(modules)

	err = ctxDb.Model(&models.ModuleStream{}).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "hash_value"}},
		DoNothing: true}).
		Create(moduleStreams).Error
	if err != nil {
		return 0, fmt.Errorf("failed to insert module streams: %w", err)
	}

	hashes := make([]string, len(moduleStreams))
	for _, m := range moduleStreams {
		hashes = append(hashes, m.HashValue)
	}
	uuids := make([]string, len(moduleStreams))

	// insert any modules streams, ignoring any hash conflicts
	if err = r.db.WithContext(ctx).
		Where("hash_value in (?)", hashes).
		Model(&models.ModuleStream{}).
		Pluck("uuid", &uuids).Error; err != nil {
		return 0, fmt.Errorf("failed retrieving existing ids in module_streams: %w", err)
	}

	// Delete repository module stream entries not needed
	err = r.deleteUnneeded(ctx, repo, uuids)
	if err != nil {
		return 0, fmt.Errorf("failed to delete unneeded module streams: %w", err)
	}

	// Add any needed repo module stream entries
	repoModStreams := make([]models.RepositoryModuleStream, len(moduleStreams))
	for i, uuid := range uuids {
		repoModStreams[i] = models.RepositoryModuleStream{
			RepositoryUUID:   repo.UUID,
			ModuleStreamUUID: uuid,
		}
	}
	err = ctxDb.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "repository_uuid"}, {Name: "module_stream_uuid"}},
		DoNothing: true}).
		Create(repoModStreams).Error
	if err != nil {
		return 0, fmt.Errorf("failed to insert repo module streams: %w", err)
	}
	return int64(len(repoModStreams)), nil
}

// deleteUnneeded removes any RepositoryPackageGroup entries that are not in the list of package_group_uuids
func (r moduleStreamsImpl) deleteUnneeded(ctx context.Context, repo models.Repository, moduleStreamUUIDs []string) error {
	if err := r.db.WithContext(ctx).Model(&models.RepositoryModuleStream{}).
		Where("repository_uuid = ?", repo.UUID).
		Where("module_stream_uuid NOT IN (?)", moduleStreamUUIDs).
		Error; err != nil {
		return err
	}
	return nil
}

func (r moduleStreamsImpl) OrphanCleanup(ctx context.Context) error {
	if err := r.db.WithContext(ctx).
		Model(&models.ModuleStream{}).
		Where("NOT EXISTS (select from repositories_module_streams where module_streams.uuid = repositories_module_streams.module_stream_uuid )").
		Delete(&models.ModuleStream{}).Error; err != nil {
		return err
	}
	return nil
}
