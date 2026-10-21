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
	Repository       *string
	SecurityLevel    *string
	Demo             bool
	EntitledFeatures []string
	Limit            int32
	Offset           int32
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
	params := store.ListLightwellPackagesParams{
		Ecosystem:     opts.Ecosystem,
		Name:          opts.Name,
		Repository:    opts.Repository,
		SecurityLevel: opts.SecurityLevel,
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
		if publishedAt >= prev.publishedAt {
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
		Group:          group,
		Name:           name,
		Versions:       itemVersions,
		LatestReleases: itemReleases,
	}
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

func (d lightwellPackageDaoImpl) SyncPackagesForRepository(ctx context.Context, repoConfigUUID string, pkgs []LightwellPackageInput) error {
	return d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		keepPkgUUIDs := make([]string, 0, len(pkgs))
		keepVerUUIDs := make([]string, 0)

		for _, in := range pkgs {
			pkg := models.LightwellPackage{
				RepositoryConfigurationUUID: repoConfigUUID,
				Name:                        in.Name,
				Group:                       in.Group,
			}
			if err := tx.Clauses(
				clause.OnConflict{
					Columns:   []clause.Column{{Name: "repository_configuration_uuid"}, {Name: "package_group"}, {Name: "name"}},
					DoUpdates: clause.AssignmentColumns([]string{"updated_at"}),
				},
				clause.Returning{Columns: []clause.Column{{Name: "uuid"}}},
			).Create(&pkg).Error; err != nil {
				return err
			}
			keepPkgUUIDs = append(keepPkgUUIDs, pkg.UUID)

			for _, v := range in.Versions {
				upstream, release := rhlw.SplitVersion(v.Version)
				ver := models.LightwellPackageVersion{
					LightwellPackageUUID:        pkg.UUID,
					RepositoryConfigurationUUID: repoConfigUUID,
					Version:                     v.Version,
					Release:                     release,
					PublishedAt:                 v.PublishedAt,
					Purl:                        v.Purl,
					UpstreamVersion:             upstream,
				}
				updates := []string{"release", "published_at", "purl", "upstream_version", "updated_at"}
				if v.Details != nil {
					ver.ProjectURL = v.Details.ProjectURL
					ver.License = v.Details.License
					ver.Summary = v.Details.Summary
					ver.Description = v.Details.Description
					ver.Author = v.Details.Author
					ver.AuthorEmail = v.Details.AuthorEmail
					updates = append(updates, "project_url", "license", "summary", "description", "author", "author_email")
				}
				if err := tx.Clauses(
					clause.OnConflict{
						Columns:   []clause.Column{{Name: "lightwell_package_uuid"}, {Name: "version"}},
						DoUpdates: clause.AssignmentColumns(updates),
					},
					clause.Returning{Columns: []clause.Column{{Name: "uuid"}}},
				).Create(&ver).Error; err != nil {
					return err
				}
				keepVerUUIDs = append(keepVerUUIDs, ver.UUID)
			}
		}

		// Delete versions that are no longer present for this repo.
		vq := tx.Where("repository_configuration_uuid = ?", repoConfigUUID)
		if len(keepVerUUIDs) > 0 {
			vq = vq.Where("uuid NOT IN ?", keepVerUUIDs)
		}
		if err := vq.Delete(&models.LightwellPackageVersion{}).Error; err != nil {
			return err
		}

		// Delete packages that are no longer present for this repo.
		pq := tx.Where("repository_configuration_uuid = ?", repoConfigUUID)
		if len(keepPkgUUIDs) > 0 {
			pq = pq.Where("uuid NOT IN ?", keepPkgUUIDs)
		}
		if err := pq.Delete(&models.LightwellPackage{}).Error; err != nil {
			return err
		}
		return nil
	})
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
		return resp, nil
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
		return resp, nil
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

	versions := make([]api.PythonPackageDetailResponse, 0, len(rows))
	for _, row := range rows {
		versions = append(versions, api.PythonPackageDetailResponse{
			Name:        row.Name,
			Version:     row.Version,
			Builds:      []api.ReleaseInfo{},
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
		return resp, nil
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

	rows, err := d.querier.ListLensCatalog(ctx, config.LightwellOrg)
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
