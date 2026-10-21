package dao

import (
	"context"
	"errors"
	"fmt"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/lightwell/db/store"
	"github.com/content-services/content-sources-backend/pkg/lightwell/rhlw"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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

func (d lightwellPackageDaoImpl) GetPythonPackageDetail(ctx context.Context, repoConfigUUID string, name string, version string) (api.PythonPackageDetailResponse, error) {
	resp := api.PythonPackageDetailResponse{}
	if d.querier == nil {
		return resp, fmt.Errorf("lightwell querier is not initialized")
	}

	repoUUID, err := uuid.Parse(repoConfigUUID)
	if err != nil {
		return resp, &ce.DaoError{BadValidation: true, Message: "invalid repository uuid"}
	}

	row, err := d.querier.GetLightwellPythonPackageDetail(ctx, store.GetLightwellPythonPackageDetailParams{
		RepositoryConfigurationUuid: repoUUID,
		Name:                        name,
		Version:                     version,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return resp, &ce.DaoError{NotFound: true, Message: "Package not found"}
	}
	if err != nil {
		return resp, fmt.Errorf("failed to get lightwell python package detail: %w", err)
	}

	if row.Versions == nil {
		row.Versions = []string{}
	}
	return api.PythonPackageDetailResponse{
		Name:        row.Name,
		Version:     row.Version,
		Summary:     row.Summary,
		Description: row.Description,
		LastUpdated: row.PublishedAt,
		License:     row.License,
		Author: api.PythonPackageAuthor{
			Name:  row.Author,
			Email: row.AuthorEmail,
		},
		UpstreamVersions: row.Versions,
		ProjectURL:       row.ProjectUrl,
		Distributions:    []api.PythonDistribution{},
	}, nil
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
