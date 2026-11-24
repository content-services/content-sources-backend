package dao

import (
	"context"
	"fmt"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/lightwell/db/store"
	"github.com/content-services/content-sources-backend/pkg/lightwell/rhlw"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/lib/pq"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// LightwellPackageVersionDetailsInput is metadata stored on the version row.
// A nil Details leaves project_url, license, summary, description, author, and author_email unchanged.
type LightwellPackageVersionDetailsInput struct {
	ProjectURL  string
	License     string
	Summary     string
	Description string
	Author      string
	AuthorEmail string
}

type LightwellPackageVersionInput struct {
	Version     string
	Release     string
	PublishedAt string
	Purl        string
	Details     *LightwellPackageVersionDetailsInput
}

type LightwellPackageInput struct {
	Name     string
	Group    string
	Versions []LightwellPackageVersionInput
}

// LightwellPackageVersionKey names one stored version of a package.
type LightwellPackageVersionKey struct {
	Group   string
	Name    string
	Version string
}

type LightwellPackageRow struct {
	RepositoryConfigurationUUID string
	RepositoryName              string
	Ecosystem                   string
	Name                        string
	Group                       string
	Versions                    []string
	Releases                    []string
	PublishedAts                []string
	UpstreamVersions            []string
	LastPublication             string
	TotalCount                  int64
}

type LightwellPackageVersionRow struct {
	RepositoryConfigurationUUID string
	RepositoryName              string
	Ecosystem                   string
	Name                        string
	Group                       string
	Version                     string
	Release                     string
	PublishedAt                 string
	UpstreamVersion             string
	Purl                        string
	TotalCount                  int64
}

type ListLightwellPackagesOptions struct {
	Ecosystem        *string
	Name             *string
	Search           *string
	Repository       *string
	SecurityLevel    *string
	Demo             bool
	EntitledFeatures []string
	// SortColumn is a normalized sort column: "group_name", "name", or "last_publication".
	// Empty defaults to "group_name".
	SortColumn string
	// SortDirection is "asc" or "desc". Empty defaults to "asc".
	SortDirection string
	Limit         int32
	Offset        int32
}

type ListLightwellPackageVersionsOptions struct {
	Ecosystem         *string
	Name              *string
	Repository        *string
	SecurityLevel     *string
	ResolvesCveID     *string
	VulnerableToCveID *string
	Demo              bool
	EntitledFeatures  []string
	Limit             int32
	Offset            int32
}

type LensCatalogPackageRow struct {
	Ecosystem       string
	Group           string
	Name            string
	Version         string
	UpstreamVersion string
}

type lightwellPackageDaoImpl struct {
	db      *gorm.DB
	querier store.Querier
}

func GetLightwellPackageDao(db *gorm.DB) LightwellPackageDao {
	return lightwellPackageDaoImpl{db: db}
}

func (d lightwellPackageDaoImpl) ListPackages(ctx context.Context, opts ListLightwellPackagesOptions) ([]LightwellPackageRow, int64, error) {
	if d.querier == nil {
		return nil, 0, fmt.Errorf("lightwell querier is not initialized")
	}
	sortColumn := opts.SortColumn
	if sortColumn == "" {
		sortColumn = "group_name"
	}
	sortDirection := opts.SortDirection
	if sortDirection == "" {
		sortDirection = "asc"
	}
	params := store.ListLightwellPackagesParams{
		Ecosystem:     opts.Ecosystem,
		Name:          opts.Name,
		Search:        opts.Search,
		Repository:    opts.Repository,
		SecurityLevel: opts.SecurityLevel,
		SortColumn:    sortColumn,
		SortDirection: sortDirection,
		DemoOrg:       config.LightwellDemoOrg,
		IsDemo:        opts.Demo,
		PageLimit:     opts.Limit,
		PageOffset:    opts.Offset,
	}
	if len(opts.EntitledFeatures) > 0 {
		params.EntitledFeatures = opts.EntitledFeatures
	}
	rows, err := d.querier.ListLightwellPackages(ctx, params)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list lightwell packages: %w", err)
	}
	var total int64
	out := make([]LightwellPackageRow, 0, len(rows))
	for _, row := range rows {
		total = row.TotalCount
		out = append(out, LightwellPackageRow{
			RepositoryConfigurationUUID: row.RepositoryConfigurationUuid.String(),
			RepositoryName:              pgtextToString(row.RepositoryName),
			Ecosystem:                   row.Ecosystem,
			Name:                        row.Name,
			Group:                       row.PackageGroup,
			Versions:                    interfaceToStringSlice(row.Versions),
			Releases:                    interfaceToStringSlice(row.Releases),
			PublishedAts:                interfaceToStringSlice(row.PublishedAts),
			UpstreamVersions:            interfaceToStringSlice(row.UpstreamVersions),
			LastPublication:             interfaceToString(row.LastPublication),
			TotalCount:                  row.TotalCount,
		})
	}
	return out, total, nil
}

func (d lightwellPackageDaoImpl) ListRepositoryPackages(ctx context.Context, repoConfigUUID string, search string, sortField string, sortDir string, limit int, offset int) (api.PackageResponse, error) {
	resp := api.PackageResponse{
		Results: []api.PackageItem{},
		Limit:   limit,
		Offset:  offset,
	}
	if d.querier == nil {
		return resp, fmt.Errorf("lightwell querier is not initialized")
	}

	repoUUID, err := uuid.Parse(repoConfigUUID)
	if err != nil {
		return resp, &ce.DaoError{BadValidation: true, Message: "invalid repository uuid"}
	}

	rows, err := d.querier.ListLightwellRepositoryPackages(ctx, store.ListLightwellRepositoryPackagesParams{
		RepositoryConfigurationUuid: repoUUID,
		Search:                      search,
		SortField:                   sortField,
		SortDir:                     sortDir,
		PageLimit:                   int32(limit),  //nolint:gosec // pagination limit is capped at MaxLimit
		PageOffset:                  int32(offset), //nolint:gosec // offset comes from parsed pagination
	})
	if err != nil {
		return resp, fmt.Errorf("failed to list lightwell repository packages: %w", err)
	}
	if len(rows) == 0 {
		return resp, nil
	}

	results := make([]api.PackageItem, 0, len(rows))
	for _, row := range rows {
		results = append(results, repositoryPackageItem(
			row.Name,
			row.PackageGroup,
			interfaceToStringSlice(row.Versions),
			interfaceToStringSlice(row.Releases),
			interfaceToStringSlice(row.PublishedAts),
			interfaceToStringSlice(row.UpstreamVersions),
		))
	}
	resp.Results = results
	resp.Total = int(rows[0].TotalCount)
	return resp, nil
}

func repositoryPackageItem(name, group string, versions, releases, publishedAts, upstreams []string) api.PackageItem {
	type latestRelease struct {
		release     string
		publishedAt string
	}
	order := make([]string, 0)
	latest := make(map[string]latestRelease)
	for i, stored := range versions {
		if stored == "" {
			continue
		}
		public := stored
		if i < len(upstreams) && upstreams[i] != "" {
			public = upstreams[i]
		}
		release := ""
		if i < len(releases) {
			release = releases[i]
		}
		publishedAt := ""
		if i < len(publishedAts) {
			publishedAt = publishedAts[i]
		}
		prev, ok := latest[public]
		if !ok {
			order = append(order, public)
			latest[public] = latestRelease{release: release, publishedAt: publishedAt}
			continue
		}
		// determine latest release by version string rather than publishedAt
		if isLaterRelease(release, publishedAt, prev.release, prev.publishedAt) {
			latest[public] = latestRelease{release: release, publishedAt: publishedAt}
		}
	}

	itemVersions := make([]string, 0, len(order))
	itemReleases := make([]api.ReleaseInfo, 0, len(order))
	for _, version := range order {
		rel := latest[version]
		itemVersions = append(itemVersions, version)
		itemReleases = append(itemReleases, api.ReleaseInfo{
			Version:   version,
			Release:   rel.release,
			CreatedAt: rel.publishedAt,
		})
	}
	return api.PackageItem{
		Group:            group,
		Name:             name,
		Versions:         itemVersions,
		UpstreamVersions: itemVersions,
		LatestReleases:   itemReleases,
	}
}

// isLaterRelease reports whether cur should replace prev as the latest build
// for one upstream version. It prefers the rhlw version string over the published_at timestamp
// (e.g. 1.2.3.rhlw-00002 published at 2026-10-01 is newer than 1.2.3.rhlw-00001 published at 2026-11-01).
// For rhlw releases the rank is compared, for non-rhlw releases it falls back to lexical compare.
// published_at is only used when ranks are equal.
func isLaterRelease(curRelease, curPublishedAt, prevRelease, prevPublishedAt string) bool {
	curRank, curOk := rhlw.Parse(curRelease)
	prevRank, prevOk := rhlw.Parse(prevRelease)
	if curOk && prevOk {
		if curRank.Baseline != prevRank.Baseline {
			return curRank.Baseline > prevRank.Baseline
		}
		if curRank.Novel != prevRank.Novel {
			return curRank.Novel > prevRank.Novel
		}
		if curRank.Hotfix != prevRank.Hotfix {
			return curRank.Hotfix > prevRank.Hotfix
		}
		return curPublishedAt >= prevPublishedAt
	}
	if curRelease != prevRelease {
		return curRelease > prevRelease
	}
	return curPublishedAt >= prevPublishedAt
}

func (d lightwellPackageDaoImpl) ListPackageVersions(ctx context.Context, opts ListLightwellPackageVersionsOptions) ([]LightwellPackageVersionRow, int64, error) {
	if d.querier == nil {
		return nil, 0, fmt.Errorf("lightwell querier is not initialized")
	}
	params := store.ListLightwellPackageVersionsParams{
		Ecosystem:         opts.Ecosystem,
		Name:              opts.Name,
		Repository:        opts.Repository,
		SecurityLevel:     opts.SecurityLevel,
		ResolvesCveID:     opts.ResolvesCveID,
		VulnerableToCveID: opts.VulnerableToCveID,
		DemoOrg:           config.LightwellDemoOrg,
		IsDemo:            opts.Demo,
		PageLimit:         opts.Limit,
		PageOffset:        opts.Offset,
	}
	if len(opts.EntitledFeatures) > 0 {
		params.EntitledFeatures = opts.EntitledFeatures
	}
	rows, err := d.querier.ListLightwellPackageVersions(ctx, params)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list lightwell package versions: %w", err)
	}
	var total int64
	out := make([]LightwellPackageVersionRow, 0, len(rows))
	for _, row := range rows {
		total = row.TotalCount
		out = append(out, LightwellPackageVersionRow{
			RepositoryConfigurationUUID: row.RepositoryConfigurationUuid.String(),
			RepositoryName:              pgtextToString(row.RepositoryName),
			Ecosystem:                   row.Ecosystem,
			Name:                        row.Name,
			Group:                       row.PackageGroup,
			Version:                     row.Version,
			Release:                     row.Release,
			PublishedAt:                 row.PublishedAt,
			UpstreamVersion:             row.UpstreamVersion,
			Purl:                        row.Purl,
			TotalCount:                  row.TotalCount,
		})
	}
	return out, total, nil
}

// lightwellPackageBatchSize keeps each package insert well under the Postgres limit of 65535 bind parameters.
const lightwellPackageBatchSize = 1000

// lightwellPackageVersionBatchSize keeps each version insert well under the Postgres limit of 65535 bind parameters.
const lightwellPackageVersionBatchSize = 500

// lightwellPackageKeyBatchSize bounds the (group, name[, version]) tuples in one delete.
const lightwellPackageKeyBatchSize = 1000

// SyncPackagesForRepository replaces the mirror for a repository with pkgs.
// Rows that are not in pkgs are deleted.
func (d lightwellPackageDaoImpl) SyncPackagesForRepository(ctx context.Context, repoConfigUUID string, pkgs []LightwellPackageInput) error {
	return d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		keepPkgUUIDs, keepVerUUIDs, err := upsertLightwellPackages(tx, repoConfigUUID, pkgs, overwriteVersion)
		if err != nil {
			return err
		}

		// Delete versions that are no longer present for this repo. The keep list is sent as
		// one array parameter, so it is not bound by the bind parameter limit.
		if err := tx.Where("repository_configuration_uuid = ?", repoConfigUUID).
			Where("NOT (uuid = ANY(?::uuid[]))", pq.Array(keepVerUUIDs)).
			Delete(&models.LightwellPackageVersion{}).Error; err != nil {
			return err
		}

		// Delete packages that are no longer present for this repo.
		if err := tx.Where("repository_configuration_uuid = ?", repoConfigUUID).
			Where("NOT (uuid = ANY(?::uuid[]))", pq.Array(keepPkgUUIDs)).
			Delete(&models.LightwellPackage{}).Error; err != nil {
			return err
		}
		return nil
	})
}

// ApplyPackageChangesForRepository updates part of the mirror for a repository.
// Versions in removals are deleted, then upserts are written. A package is deleted when a
// removal leaves it with no versions. Rows that are in neither list are not touched.
// An upserted version that is already stored keeps its metadata, because Pulp content is
// immutable and every file of a version carries the same release metadata. Only its
// published_at moves forward when the upsert is later.
func (d lightwellPackageDaoImpl) ApplyPackageChangesForRepository(ctx context.Context, repoConfigUUID string, upserts []LightwellPackageInput, removals []LightwellPackageVersionKey) error {
	if len(upserts) == 0 && len(removals) == 0 {
		return nil
	}
	return d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := deleteLightwellPackageVersions(tx, repoConfigUUID, removals); err != nil {
			return err
		}
		if _, _, err := upsertLightwellPackages(tx, repoConfigUUID, upserts, keepVersion); err != nil {
			return err
		}
		return deleteEmptyLightwellPackages(tx, repoConfigUUID, removals)
	})
}

// versionConflict says what an upsert does to a version row that is already stored.
type versionConflict int

const (
	// overwriteVersion replaces the stored row. A version with nil Details leaves the stored metadata unchanged.
	overwriteVersion versionConflict = iota
	// keepVersion keeps the stored row, and only moves published_at forward when the new value is later.
	keepVersion
)

// laterPublishedAt picks the later of the stored and the new published_at. The values are RFC3339
// text written by different importers, so they are compared as timestamps. An empty value is the earliest.
const laterPublishedAt = `CASE
	WHEN COALESCE(NULLIF(excluded.published_at, '')::timestamptz, '-infinity') >
	     COALESCE(NULLIF(lightwell_package_versions.published_at, '')::timestamptz, '-infinity')
	THEN excluded.published_at
	ELSE lightwell_package_versions.published_at
END`

// upsertLightwellPackages writes packages and versions in batches and returns the uuids of every row written.
// Repeated packages are merged. A repeated version keeps its last entry.
// conflict decides what happens to a version that is already stored.
func upsertLightwellPackages(tx *gorm.DB, repoConfigUUID string, pkgs []LightwellPackageInput, conflict versionConflict) ([]string, []string, error) {
	type packageKey struct{ group, name string }
	order := make([]packageKey, 0, len(pkgs))
	versionsByPkg := make(map[packageKey][]LightwellPackageVersionInput, len(pkgs))
	for _, in := range pkgs {
		key := packageKey{group: in.Group, name: in.Name}
		if _, ok := versionsByPkg[key]; !ok {
			order = append(order, key)
		}
		versionsByPkg[key] = append(versionsByPkg[key], in.Versions...)
	}

	pkgUUIDs := make([]string, 0, len(order))
	verUUIDs := make([]string, 0)
	for start := 0; start < len(order); start += lightwellPackageBatchSize {
		keys := order[start:min(start+lightwellPackageBatchSize, len(order))]
		rows := make([]models.LightwellPackage, len(keys))
		for i, key := range keys {
			rows[i] = models.LightwellPackage{
				RepositoryConfigurationUUID: repoConfigUUID,
				Name:                        key.name,
				Group:                       key.group,
			}
		}
		if err := tx.Clauses(
			clause.OnConflict{
				Columns:   []clause.Column{{Name: "repository_configuration_uuid"}, {Name: "package_group"}, {Name: "name"}},
				DoUpdates: clause.AssignmentColumns([]string{"updated_at"}),
			},
			clause.Returning{Columns: []clause.Column{{Name: "uuid"}}},
		).Create(&rows).Error; err != nil {
			return nil, nil, err
		}

		var withDetails, withoutDetails []models.LightwellPackageVersion
		for i, key := range keys {
			pkgUUIDs = append(pkgUUIDs, rows[i].UUID)
			for _, v := range dedupeLightwellPackageVersions(versionsByPkg[key]) {
				ver := newLightwellPackageVersion(rows[i].UUID, repoConfigUUID, v)
				if v.Details != nil {
					withDetails = append(withDetails, ver)
				} else {
					withoutDetails = append(withoutDetails, ver)
				}
			}
		}
		written, err := upsertLightwellPackageVersions(tx, withDetails, true, conflict)
		if err != nil {
			return nil, nil, err
		}
		verUUIDs = append(verUUIDs, written...)
		written, err = upsertLightwellPackageVersions(tx, withoutDetails, false, conflict)
		if err != nil {
			return nil, nil, err
		}
		verUUIDs = append(verUUIDs, written...)
	}
	return pkgUUIDs, verUUIDs, nil
}

// dedupeLightwellPackageVersions keeps the last entry for each version.
// One insert cannot update the same row twice.
func dedupeLightwellPackageVersions(versions []LightwellPackageVersionInput) []LightwellPackageVersionInput {
	index := make(map[string]int, len(versions))
	out := make([]LightwellPackageVersionInput, 0, len(versions))
	for _, v := range versions {
		if i, ok := index[v.Version]; ok {
			out[i] = v
			continue
		}
		index[v.Version] = len(out)
		out = append(out, v)
	}
	return out
}

func newLightwellPackageVersion(pkgUUID, repoConfigUUID string, v LightwellPackageVersionInput) models.LightwellPackageVersion {
	upstream, release := rhlw.SplitVersion(v.Version)
	ver := models.LightwellPackageVersion{
		LightwellPackageUUID:        pkgUUID,
		RepositoryConfigurationUUID: repoConfigUUID,
		Version:                     v.Version,
		Release:                     release,
		PublishedAt:                 v.PublishedAt,
		Purl:                        v.Purl,
		UpstreamVersion:             upstream,
	}
	if v.Details != nil {
		ver.ProjectURL = v.Details.ProjectURL
		ver.License = v.Details.License
		ver.Summary = v.Details.Summary
		ver.Description = v.Details.Description
		ver.Author = v.Details.Author
		ver.AuthorEmail = v.Details.AuthorEmail
	}
	return ver
}

// upsertLightwellPackageVersions writes versions in batches. Every row in one call must agree on
// whether it carries metadata, because one insert has one ON CONFLICT update list.
func upsertLightwellPackageVersions(tx *gorm.DB, versions []models.LightwellPackageVersion, withDetails bool, conflict versionConflict) ([]string, error) {
	updates := []string{"release", "published_at", "purl", "upstream_version", "updated_at"}
	if withDetails {
		updates = append(updates, "project_url", "license", "summary", "description", "author", "author_email")
	}
	onConflict := clause.AssignmentColumns(updates)
	if conflict == keepVersion {
		onConflict = clause.Set{
			{Column: clause.Column{Name: "published_at"}, Value: gorm.Expr(laterPublishedAt)},
			{Column: clause.Column{Name: "updated_at"}, Value: gorm.Expr("excluded.updated_at")},
		}
	}
	uuids := make([]string, 0, len(versions))
	for start := 0; start < len(versions); start += lightwellPackageVersionBatchSize {
		batch := versions[start:min(start+lightwellPackageVersionBatchSize, len(versions))]
		if err := tx.Clauses(
			clause.OnConflict{
				Columns:   []clause.Column{{Name: "lightwell_package_uuid"}, {Name: "version"}},
				DoUpdates: onConflict,
			},
			clause.Returning{Columns: []clause.Column{{Name: "uuid"}}},
		).Create(&batch).Error; err != nil {
			return nil, err
		}
		for _, ver := range batch {
			uuids = append(uuids, ver.UUID)
		}
	}
	return uuids, nil
}

func deleteLightwellPackageVersions(tx *gorm.DB, repoConfigUUID string, keys []LightwellPackageVersionKey) error {
	for start := 0; start < len(keys); start += lightwellPackageKeyBatchSize {
		batch := keys[start:min(start+lightwellPackageKeyBatchSize, len(keys))]
		tuples := make([][]any, len(batch))
		for i, key := range batch {
			tuples[i] = []any{key.Group, key.Name, key.Version}
		}
		if err := tx.Exec(`
			DELETE FROM lightwell_package_versions v
			USING lightwell_packages p
			WHERE v.lightwell_package_uuid = p.uuid
				AND p.repository_configuration_uuid = ?
				AND (p.package_group, p.name, v.version) IN ?`,
			repoConfigUUID, tuples,
		).Error; err != nil {
			return err
		}
	}
	return nil
}

// deleteEmptyLightwellPackages deletes the packages named in keys that have no versions left.
func deleteEmptyLightwellPackages(tx *gorm.DB, repoConfigUUID string, keys []LightwellPackageVersionKey) error {
	type packageKey struct{ group, name string }
	seen := make(map[packageKey]struct{}, len(keys))
	tuples := make([][]any, 0, len(keys))
	for _, key := range keys {
		pk := packageKey{group: key.Group, name: key.Name}
		if _, ok := seen[pk]; ok {
			continue
		}
		seen[pk] = struct{}{}
		tuples = append(tuples, []any{key.Group, key.Name})
	}
	for start := 0; start < len(tuples); start += lightwellPackageKeyBatchSize {
		batch := tuples[start:min(start+lightwellPackageKeyBatchSize, len(tuples))]
		if err := tx.Exec(`
			DELETE FROM lightwell_packages p
			WHERE p.repository_configuration_uuid = ?
				AND (p.package_group, p.name) IN ?
				AND NOT EXISTS (
					SELECT 1 FROM lightwell_package_versions v WHERE v.lightwell_package_uuid = p.uuid
				)`,
			repoConfigUUID, batch,
		).Error; err != nil {
			return err
		}
	}
	return nil
}

// InternalOnlyListStoredPackageVersions returns the version strings stored for one package.
// The match is exact on package group and name. Python callers pass a normalized name and an empty group.
func (d lightwellPackageDaoImpl) InternalOnlyListStoredPackageVersions(ctx context.Context, repoConfigUUID, group, name string) ([]string, error) {
	if d.querier == nil {
		return nil, fmt.Errorf("lightwell querier is not initialized")
	}
	repoUUID, err := uuid.Parse(repoConfigUUID)
	if err != nil {
		return nil, fmt.Errorf("repository configuration uuid is required")
	}

	versions, err := d.querier.ListStoredPackageVersions(ctx, store.ListStoredPackageVersionsParams{
		RepositoryConfigurationUuid: repoUUID,
		PackageGroup:                group,
		Name:                        name,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list stored package versions: %w", err)
	}
	return versions, nil
}

// InternalOnlyCountStoredContent counts the mirror rows for one repository configuration.
// Package count is the number of packages. Version count is the number of distinct
// group, name, and upstream version combinations. Build count is the number of version rows.
func (d lightwellPackageDaoImpl) InternalOnlyCountStoredContent(ctx context.Context, repoConfigUUID string) (int, int, int, error) {
	if d.querier == nil {
		return 0, 0, 0, fmt.Errorf("lightwell querier is not initialized")
	}
	repoUUID, err := uuid.Parse(repoConfigUUID)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("repository configuration uuid is required")
	}

	counts, err := d.querier.CountStoredPackageContent(ctx, repoUUID)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("failed to count stored package content: %w", err)
	}
	return int(counts.PackageCount), int(counts.BuildCount), int(counts.VersionCount), nil
}

func (d lightwellPackageDaoImpl) ListMavenPackageVersions(ctx context.Context, repoConfigUUID string, group string, name string) (api.MavenPackageVersionsResponse, error) {
	resp := api.MavenPackageVersionsResponse{Group: group, Name: name, Versions: []api.MavenPackageDetailResponse{}}
	if d.querier == nil {
		return resp, fmt.Errorf("lightwell querier is not initialized")
	}

	repoUUID, err := uuid.Parse(repoConfigUUID)
	if err != nil {
		return resp, &ce.DaoError{BadValidation: true, Message: "invalid repository uuid"}
	}

	rows, err := d.querier.ListLightwellMavenPackageVersions(ctx, store.ListLightwellMavenPackageVersionsParams{
		RepositoryConfigurationUuid: repoUUID,
		PackageGroup:                group,
		Name:                        name,
	})
	if err != nil {
		return resp, fmt.Errorf("failed to list lightwell maven package versions: %w", err)
	}
	if len(rows) == 0 {
		return resp, &ce.DaoError{NotFound: true, Message: "Package not found"}
	}

	versions := make([]api.MavenPackageDetailResponse, 0)
	index := make(map[string]int, len(rows))
	for _, row := range rows {
		version := row.UpstreamVersion
		if version == "" {
			version = row.Version
		}
		build := api.ReleaseInfo{
			Version:   version,
			Release:   row.Release,
			CreatedAt: row.PublishedAt,
		}
		if i, ok := index[version]; ok {
			versions[i].Builds = append(versions[i].Builds, build)
			continue
		}
		index[version] = len(versions)
		detail := api.MavenPackageDetailResponse{
			Group:           row.PackageGroup,
			Name:            row.Name,
			Version:         version,
			LastUpdated:     row.PublishedAt,
			UpstreamVersion: row.UpstreamVersion,
			Builds:          []api.ReleaseInfo{build},
		}
		fillMavenPackageMetadata(&detail, row.Summary, row.License, row.ProjectUrl, row.Author)
		versions = append(versions, detail)
	}
	if len(rows) > 0 {
		resp.Group = rows[0].PackageGroup
		resp.Name = rows[0].Name
	}
	resp.Versions = versions
	return resp, nil
}

func fillMavenPackageMetadata(dst *api.MavenPackageDetailResponse, summary, license, projectURL, author string) {
	dst.Summary = optionalString(summary)
	dst.License = optionalString(license)
	dst.ProjectURL = optionalString(projectURL)
	dst.Author = optionalString(author)
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func (d lightwellPackageDaoImpl) GetMavenPackageDetail(ctx context.Context, repoConfigUUID string, group string, name string, version string) (api.MavenPackageDetailResponse, error) {
	resp := api.MavenPackageDetailResponse{
		Group:   group,
		Name:    name,
		Version: version,
		Builds:  []api.ReleaseInfo{},
	}
	if d.querier == nil {
		return resp, fmt.Errorf("lightwell querier is not initialized")
	}

	repoUUID, err := uuid.Parse(repoConfigUUID)
	if err != nil {
		return resp, &ce.DaoError{BadValidation: true, Message: "invalid repository uuid"}
	}

	rows, err := d.querier.ListLightwellMavenPackageDetail(ctx, store.ListLightwellMavenPackageDetailParams{
		RepositoryConfigurationUuid: repoUUID,
		PackageGroup:                group,
		Name:                        name,
		Version:                     version,
	})
	if err != nil {
		return resp, fmt.Errorf("failed to get lightwell maven package detail: %w", err)
	}
	if len(rows) == 0 {
		return resp, &ce.DaoError{NotFound: true, Message: "Package not found"}
	}

	builds := make([]api.ReleaseInfo, 0, len(rows))
	for _, row := range rows {
		publicVersion := row.UpstreamVersion
		if publicVersion == "" {
			publicVersion = row.Version
		}
		builds = append(builds, api.ReleaseInfo{
			Version:   publicVersion,
			Release:   row.Release,
			CreatedAt: row.PublishedAt,
		})
	}
	resp.Group = rows[0].PackageGroup
	resp.Name = rows[0].Name
	resp.LastUpdated = rows[0].PublishedAt
	resp.UpstreamVersion = rows[0].UpstreamVersion
	fillMavenPackageMetadata(&resp, rows[0].Summary, rows[0].License, rows[0].ProjectUrl, rows[0].Author)
	resp.Builds = builds
	return resp, nil
}

func (d lightwellPackageDaoImpl) ListNpmPackageVersions(ctx context.Context, repoConfigUUID string, scope string, name string) (api.NpmPackageVersionsResponse, error) {
	resp := api.NpmPackageVersionsResponse{Scope: scope, Name: name, Versions: []api.NpmPackageDetailResponse{}}
	if d.querier == nil {
		return resp, fmt.Errorf("lightwell querier is not initialized")
	}

	repoUUID, err := uuid.Parse(repoConfigUUID)
	if err != nil {
		return resp, &ce.DaoError{BadValidation: true, Message: "invalid repository uuid"}
	}

	rows, err := d.querier.ListLightwellNpmPackageVersions(ctx, store.ListLightwellNpmPackageVersionsParams{
		RepositoryConfigurationUuid: repoUUID,
		PackageGroup:                scope,
		Name:                        name,
	})
	if err != nil {
		return resp, fmt.Errorf("failed to list lightwell npm package versions: %w", err)
	}
	if len(rows) == 0 {
		return resp, &ce.DaoError{NotFound: true, Message: "Package not found"}
	}

	versions := make([]api.NpmPackageDetailResponse, 0)
	index := make(map[string]int, len(rows))
	for _, row := range rows {
		version := row.UpstreamVersion
		if version == "" {
			version = row.Version
		}
		build := api.ReleaseInfo{
			Version:   version,
			Release:   row.Release,
			CreatedAt: row.PublishedAt,
		}
		if i, ok := index[version]; ok {
			versions[i].Builds = append(versions[i].Builds, build)
			continue
		}
		index[version] = len(versions)
		versions = append(versions, api.NpmPackageDetailResponse{
			Scope:           row.PackageGroup,
			Name:            row.Name,
			Version:         version,
			LastUpdated:     row.PublishedAt,
			UpstreamVersion: row.UpstreamVersion,
			Builds:          []api.ReleaseInfo{build},
		})
	}
	if len(rows) > 0 {
		resp.Scope = rows[0].PackageGroup
		resp.Name = rows[0].Name
	}
	resp.Versions = versions
	return resp, nil
}

func (d lightwellPackageDaoImpl) GetNpmPackageDetail(ctx context.Context, repoConfigUUID string, scope string, name string, version string) (api.NpmPackageDetailResponse, error) {
	resp := api.NpmPackageDetailResponse{
		Scope:   scope,
		Name:    name,
		Version: version,
		Builds:  []api.ReleaseInfo{},
	}
	if d.querier == nil {
		return resp, fmt.Errorf("lightwell querier is not initialized")
	}

	repoUUID, err := uuid.Parse(repoConfigUUID)
	if err != nil {
		return resp, &ce.DaoError{BadValidation: true, Message: "invalid repository uuid"}
	}

	rows, err := d.querier.ListLightwellNpmPackageDetail(ctx, store.ListLightwellNpmPackageDetailParams{
		RepositoryConfigurationUuid: repoUUID,
		PackageGroup:                scope,
		Name:                        name,
		Version:                     version,
	})
	if err != nil {
		return resp, fmt.Errorf("failed to get lightwell npm package detail: %w", err)
	}
	if len(rows) == 0 {
		return resp, &ce.DaoError{NotFound: true, Message: "Package not found"}
	}

	builds := make([]api.ReleaseInfo, 0, len(rows))
	for _, row := range rows {
		publicVersion := row.UpstreamVersion
		if publicVersion == "" {
			publicVersion = row.Version
		}
		builds = append(builds, api.ReleaseInfo{
			Version:   publicVersion,
			Release:   row.Release,
			CreatedAt: row.PublishedAt,
		})
	}
	resp.Scope = rows[0].PackageGroup
	resp.Name = rows[0].Name
	resp.LastUpdated = rows[0].PublishedAt
	resp.UpstreamVersion = rows[0].UpstreamVersion
	resp.Builds = builds
	return resp, nil
}

func (d lightwellPackageDaoImpl) ListPythonPackageVersions(ctx context.Context, repoConfigUUID string, name string) (api.PythonPackageVersionsResponse, error) {
	resp := api.PythonPackageVersionsResponse{Name: name, Versions: []api.PythonPackageDetailResponse{}}
	if d.querier == nil {
		return resp, fmt.Errorf("lightwell querier is not initialized")
	}

	repoUUID, err := uuid.Parse(repoConfigUUID)
	if err != nil {
		return resp, &ce.DaoError{BadValidation: true, Message: "invalid repository uuid"}
	}

	rows, err := d.querier.ListLightwellPythonPackageVersions(ctx, store.ListLightwellPythonPackageVersionsParams{
		RepositoryConfigurationUuid: repoUUID,
		Name:                        name,
	})
	if err != nil {
		return resp, fmt.Errorf("failed to list lightwell python package versions: %w", err)
	}
	if len(rows) == 0 {
		return resp, &ce.DaoError{NotFound: true, Message: "Package not found"}
	}

	versions := make([]api.PythonPackageDetailResponse, 0)
	index := make(map[string]int, len(rows))
	for _, row := range rows {
		version := row.UpstreamVersion
		if version == "" {
			version = row.Version
		}
		build := api.ReleaseInfo{
			Version:   version,
			Release:   row.Release,
			CreatedAt: row.PublishedAt,
		}
		if i, ok := index[version]; ok {
			versions[i].Builds = append(versions[i].Builds, build)
			continue
		}
		index[version] = len(versions)
		versions = append(versions, api.PythonPackageDetailResponse{
			Name:        row.Name,
			Version:     version,
			Builds:      []api.ReleaseInfo{build},
			Summary:     row.Summary,
			Description: row.Description,
			LastUpdated: row.PublishedAt,
			License:     row.License,
			Author: api.PythonPackageAuthor{
				Name:  row.Author,
				Email: row.AuthorEmail,
			},
			UpstreamVersion: row.UpstreamVersion,
			ProjectURL:      row.ProjectUrl,
			Distributions:   []api.PythonDistribution{},
		})
	}
	resp.Name = rows[0].Name
	resp.Versions = versions
	return resp, nil
}

func (d lightwellPackageDaoImpl) GetPythonPackageDetail(ctx context.Context, repoConfigUUID string, name string, version string) (api.PythonPackageDetailResponse, error) {
	resp := api.PythonPackageDetailResponse{
		Name:          name,
		Version:       version,
		Builds:        []api.ReleaseInfo{},
		Distributions: []api.PythonDistribution{},
	}
	if d.querier == nil {
		return resp, fmt.Errorf("lightwell querier is not initialized")
	}

	repoUUID, err := uuid.Parse(repoConfigUUID)
	if err != nil {
		return resp, &ce.DaoError{BadValidation: true, Message: "invalid repository uuid"}
	}

	rows, err := d.querier.ListLightwellPythonPackageDetail(ctx, store.ListLightwellPythonPackageDetailParams{
		RepositoryConfigurationUuid: repoUUID,
		Name:                        name,
		Version:                     version,
	})
	if err != nil {
		return resp, fmt.Errorf("failed to get lightwell python package detail: %w", err)
	}
	if len(rows) == 0 {
		return resp, &ce.DaoError{NotFound: true, Message: "Package not found"}
	}

	builds := make([]api.ReleaseInfo, 0, len(rows))
	for _, row := range rows {
		publicVersion := row.UpstreamVersion
		if publicVersion == "" {
			publicVersion = row.Version
		}
		builds = append(builds, api.ReleaseInfo{
			Version:   publicVersion,
			Release:   row.Release,
			CreatedAt: row.PublishedAt,
		})
	}
	resp.Name = rows[0].Name
	resp.Summary = rows[0].Summary
	resp.Description = rows[0].Description
	resp.LastUpdated = rows[0].PublishedAt
	resp.License = rows[0].License
	resp.Author = api.PythonPackageAuthor{Name: rows[0].Author, Email: rows[0].AuthorEmail}
	resp.UpstreamVersion = rows[0].UpstreamVersion
	resp.ProjectURL = rows[0].ProjectUrl
	resp.Builds = builds
	return resp, nil
}

func (d lightwellPackageDaoImpl) InternalOnlyListLensCatalog(ctx context.Context) ([]LensCatalogPackageRow, error) {
	if d.querier == nil {
		return nil, fmt.Errorf("lightwell querier is not initialized")
	}

	rows, err := d.querier.ListLensCatalog(ctx, store.ListLensCatalogParams{
		OrgID:        config.LightwellOrg,
		IgnoredNames: append([]string{}, config.Get().Options.LightwellIgnoreRepos...),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list lens catalog: %w", err)
	}

	out := make([]LensCatalogPackageRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, LensCatalogPackageRow{
			Ecosystem:       row.Ecosystem,
			Group:           row.PackageGroup,
			Name:            row.Name,
			Version:         row.Version,
			UpstreamVersion: row.UpstreamVersion,
		})
	}
	return out, nil
}

func pgtextToString(t pgtype.Text) string {
	if !t.Valid {
		return ""
	}
	return t.String
}

func interfaceToString(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func interfaceToStringSlice(v interface{}) []string {
	if v == nil {
		return []string{}
	}
	switch arr := v.(type) {
	case []interface{}:
		result := make([]string, 0, len(arr))
		for _, item := range arr {
			if item == nil {
				continue
			}
			if str, ok := item.(string); ok {
				result = append(result, str)
			}
		}
		return result
	case []string:
		return arr
	default:
		return []string{}
	}
}
