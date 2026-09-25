package dao

import (
	"context"
	"fmt"

	"github.com/content-services/content-sources-backend/pkg/lightwell/db/store"
	"github.com/jackc/pgx/v5/pgtype"
	"gorm.io/gorm"
)

// TODO(Task 5): replace stub
type LightwellPackageInput struct{}

type LightwellPackageRow struct {
	RepositoryConfigurationUUID string
	RepositoryName              string
	Ecosystem                   string
	Name                        string
	Group                       string
	Versions                    []string
	Releases                    []string
	PublishedAts                []string
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
	Purl                        string
	TotalCount                  int64
}

type ListLightwellPackagesOptions struct {
	Ecosystem        *string
	Name             *string
	Repository       *string
	SecurityLevel    *string
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
	EntitledFeatures  []string
	Limit             int32
	Offset            int32
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
			Purl:                        row.Purl,
			TotalCount:                  row.TotalCount,
		})
	}
	return out, total, nil
}

// TODO(Task 5): replace stub
func (d lightwellPackageDaoImpl) SyncPackagesForRepository(ctx context.Context, repoConfigUUID string, pkgs []LightwellPackageInput) error {
	return fmt.Errorf("not implemented")
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
		result := make([]string, len(arr))
		for i, item := range arr {
			if str, ok := item.(string); ok {
				result[i] = str
			}
		}
		return result
	case []string:
		return arr
	default:
		return []string{}
	}
}
