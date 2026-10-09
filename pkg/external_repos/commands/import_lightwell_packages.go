package commands

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/handler"
	"github.com/content-services/content-sources-backend/pkg/lightwell/coords"
	"github.com/content-services/content-sources-backend/pkg/lightwell/rhlw"
	"github.com/content-services/tang/pkg/tangy"
	zest "github.com/content-services/zest/release/v2026"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v2"
	"gorm.io/gorm"
)

const importMaxConcurrentRepos = 10
const importPageSize = 300

// loadMavenCentralMetadata is replaced in tests so the import does not call Maven Central.
var loadMavenCentralMetadata = handler.FetchMavenCentralMetadata

func ImportLightwellPackagesAction(c *cli.Context) error {
	ctx := c.Context
	force := c.Bool("force")
	if config.Tang == nil {
		if err := config.ConfigureTang(); err != nil {
			return fmt.Errorf("failed to configure tang: %w", err)
		}
	}
	if err := importLightwellPackages(ctx, db.DB, force); err != nil {
		log.Error().Err(err).Msg("Failed to import lightwell packages")
		return err
	}
	log.Info().Msg("Successfully imported lightwell packages.")
	return nil
}

func importLightwellPackages(ctx context.Context, database *gorm.DB, force bool) error {
	daoReg := dao.GetDaoRegistry(database)
	repos, err := daoReg.RepositoryConfig.InternalOnly_ListLightwellReposToImport(ctx)
	if err != nil {
		return fmt.Errorf("error listing lightwell repos: %w", err)
	}

	errsByIdx := make([]error, len(repos))
	var wg sync.WaitGroup
	sem := make(chan struct{}, importMaxConcurrentRepos)

	for i, repo := range repos {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, r dao.LightwellRepoToImport) {
			defer wg.Done()
			defer func() { <-sem }()

			domain, err := daoReg.Domain.FetchOrCreateDomain(ctx, r.OrgID)
			if err != nil {
				log.Error().Err(err).Str("repo", r.Name).Msg("Failed to fetch domain, continuing")
				errsByIdx[idx] = fmt.Errorf("repo %s: %w", r.Name, err)
				return
			}
			pulpClient := pulp_client.GetPulpClientWithDomain(domain)

			if err := importRepo(ctx, daoReg, pulpClient, r, force); err != nil {
				log.Error().Err(err).Str("repo", r.Name).Msg("Failed to import lightwell repo, continuing")
				errsByIdx[idx] = fmt.Errorf("repo %s: %w", r.Name, err)
			}
		}(i, repo)
	}
	wg.Wait()

	var errs []error
	for _, e := range errsByIdx {
		if e != nil {
			errs = append(errs, e)
		}
	}
	return errors.Join(errs...)
}

func importRepo(ctx context.Context, daoReg *dao.DaoRegistry, pulpClient pulp_client.PulpClient, repo dao.LightwellRepoToImport, force bool) error {
	if repo.BasePath == "" {
		return nil
	}

	repoHref, err := pulpClient.ResolveRepositoryFromBasePath(ctx, repo.BasePath)
	if err != nil {
		return err
	}
	if repoHref == nil {
		log.Warn().Str("repo", repo.Name).Msg("distribution not found, skipping")
		return nil
	}

	currentVersion, err := pulpClient.GetLatestVersionHref(ctx, *repoHref)
	if err != nil {
		return err
	}
	if !shouldImport(currentVersion, repo.LastImportRepositoryVersion, force) {
		return nil
	}

	pkgs, err := fetchRepoPackages(ctx, daoReg.MavenPackages, pulpClient, repo.ContentType, *repoHref)
	if err != nil {
		return err
	}
	if err := daoReg.LightwellPackage.SyncPackagesForRepository(ctx, repo.RepoConfigUUID, pkgs); err != nil {
		return err
	}
	return daoReg.RepositoryConfig.InternalOnly_UpdateLastImportRepositoryVersion(ctx, repo.RepoConfigUUID, *currentVersion)
}

func shouldImport(current *string, last string, force bool) bool {
	if current == nil {
		return false
	}
	if force {
		return true
	}
	return *current != last
}

func fetchRepoPackages(ctx context.Context, packages dao.MavenPackagesDao, pulpClient pulp_client.PulpClient, contentType, repoHref string) ([]dao.LightwellPackageInput, error) {
	switch contentType {
	case config.ContentTypeMaven:
		resp, err := pulpClient.ListMavenFlatPackages(ctx, repoHref)
		if err != nil {
			return nil, err
		}
		return mapMavenFlatPackageInputs(resp.Results, newMavenCentralLookup(ctx, packages, loadMavenCentralMetadata)), nil
	case config.ContentTypePython:
		if config.Tang == nil {
			return nil, fmt.Errorf("tang is not configured")
		}
		var all []dao.LightwellPackageInput
		offset := 0
		// PythonPackageDetailList rejects a limit above its max. Pagination is by package.
		limit := tangy.PythonPackageDetailListMaxLimit
		for {
			resp, err := (*config.Tang).PythonPackageDetailList(ctx, repoHref, tangy.PageOptions{Offset: offset, Limit: limit})
			if err != nil {
				return nil, err
			}
			all = append(all, mapPythonPackageInputs(resp)...)
			offset += len(resp.Results)
			if len(resp.Results) == 0 || offset >= resp.Total {
				break
			}
		}
		return all, nil
	case config.ContentTypeNpm:
		if config.Tang == nil {
			return nil, fmt.Errorf("tang is not configured")
		}
		var all []dao.LightwellPackageInput
		offset := 0
		for {
			resp, err := (*config.Tang).NpmPackageList(ctx, repoHref, tangy.NpmPackageListFilters{}, tangy.PageOptions{Offset: offset, Limit: importPageSize})
			if err != nil {
				return nil, err
			}
			all = append(all, mapNpmPackageInputs(resp)...)
			offset += len(resp.Results)
			if len(resp.Results) == 0 || offset >= resp.Total {
				break
			}
		}
		return all, nil
	default:
		return nil, nil
	}
}

type mavenCentralLookup struct {
	ctx      context.Context
	packages dao.MavenPackagesDao
	load     func(context.Context, string, string, string) (handler.MavenCentralMetadata, error)
	cache    map[string]*dao.LightwellPackageVersionDetailsInput
	// local is keyed by group and artifact. maven_packages has one row per coordinate, not per version.
	local map[string]cachedMavenPackage
}

type cachedMavenPackage struct {
	found   bool
	details *dao.LightwellPackageVersionDetailsInput
}

func newMavenCentralLookup(ctx context.Context, packages dao.MavenPackagesDao, load func(context.Context, string, string, string) (handler.MavenCentralMetadata, error)) *mavenCentralLookup {
	return &mavenCentralLookup{
		ctx:      ctx,
		packages: packages,
		load:     load,
		cache:    map[string]*dao.LightwellPackageVersionDetailsInput{},
		local:    map[string]cachedMavenPackage{},
	}
}

// details loads text for one group, artifact, and upstream version.
// Request-path reads still fill maven_packages and fall through to Maven Central.
// That path will be removed, and Maven Central will be called only from this sync.
// Until then, use a maven_packages row when one exists and call Central only when it does not.
// A read error is logged and treated as a miss. A missing POM falls back to the flat row.
// Any other Central error writes empty text.
func (l *mavenCentralLookup) details(group, artifact, upstream string, row zest.MavenRepositoryFlatPackageResponse) *dao.LightwellPackageVersionDetailsInput {
	key := group + "\x00" + artifact + "\x00" + upstream
	if got, ok := l.cache[key]; ok {
		return got
	}

	if l.packages != nil {
		if local := l.mavenPackageDetails(group, artifact); local != nil {
			l.cache[key] = local
			return local
		}
	}

	var details *dao.LightwellPackageVersionDetailsInput
	meta, err := l.load(l.ctx, group, artifact, upstream)
	switch {
	case err == nil:
		details = &dao.LightwellPackageVersionDetailsInput{
			ProjectURL:  meta.ProjectURL,
			License:     meta.License,
			Summary:     meta.Summary,
			Description: meta.Description,
			Author:      meta.Author,
			AuthorEmail: meta.AuthorEmail,
		}
	case handler.IsMavenCentralNotFound(err):
		details = &dao.LightwellPackageVersionDetailsInput{
			Summary:     row.Description,
			Description: row.Description,
			License:     joinMavenLicenseNames(row.Licenses),
		}
	default:
		log.Warn().Err(err).Str("group_id", group).Str("artifact_id", artifact).Str("upstream_version", upstream).Msg("failed to fetch maven central metadata")
		details = &dao.LightwellPackageVersionDetailsInput{}
	}
	l.cache[key] = details
	return details
}

// mavenPackageDetails returns the cached maven_packages row for a group and artifact.
// A second call for the same coordinate does not read the table again.
// The table stores the POM description in summary, so both summary and description get that text.
// A read error is logged once for this coordinate and then treated as a miss.
func (l *mavenCentralLookup) mavenPackageDetails(group, artifact string) *dao.LightwellPackageVersionDetailsInput {
	key := group + "\x00" + artifact
	if got, ok := l.local[key]; ok {
		if !got.found {
			return nil
		}
		return got.details
	}

	existing, err := l.packages.Fetch(l.ctx, group, artifact)
	if err != nil {
		log.Warn().Err(err).Str("group_id", group).Str("artifact_id", artifact).Msg("failed to read maven_packages, falling back to maven central")
		l.local[key] = cachedMavenPackage{}
		return nil
	}
	if existing == nil {
		l.local[key] = cachedMavenPackage{}
		return nil
	}

	summary := stringOrEmpty(existing.Summary)
	details := &dao.LightwellPackageVersionDetailsInput{
		ProjectURL:  stringOrEmpty(existing.ProjectURL),
		License:     stringOrEmpty(existing.License),
		Summary:     summary,
		Description: summary,
		Author:      stringOrEmpty(existing.Author),
	}
	l.local[key] = cachedMavenPackage{found: true, details: details}
	return details
}

func stringOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func joinMavenLicenseNames(licenses []zest.MavenPackageLicenseResponse) string {
	names := make([]string, 0, len(licenses))
	for _, lic := range licenses {
		if name := strings.TrimSpace(lic.Name); name != "" {
			names = append(names, name)
		}
	}
	return strings.Join(names, ", ")
}

func mapMavenFlatPackageInputs(rows []zest.MavenRepositoryFlatPackageResponse, lookup *mavenCentralLookup) []dao.LightwellPackageInput {
	type packageKey struct {
		group    string
		artifact string
	}
	order := make([]packageKey, 0)
	grouped := make(map[packageKey][]zest.MavenRepositoryFlatPackageResponse)
	for _, row := range rows {
		key := packageKey{group: row.GroupId, artifact: row.ArtifactId}
		if _, ok := grouped[key]; !ok {
			order = append(order, key)
		}
		grouped[key] = append(grouped[key], row)
	}

	out := make([]dao.LightwellPackageInput, 0, len(order))
	for _, key := range order {
		groupRows := grouped[key]
		versions := make([]dao.LightwellPackageVersionInput, 0, len(groupRows))
		for _, row := range groupRows {
			upstream, _ := rhlw.SplitVersion(row.Version)
			versions = append(versions, dao.LightwellPackageVersionInput{
				Version:     row.Version,
				PublishedAt: row.LastUpdated.UTC().Format(time.RFC3339),
				Purl:        coords.BuildPURL(config.ContentTypeMaven, row.GroupId, row.ArtifactId, row.Version),
				Details:     lookup.details(row.GroupId, row.ArtifactId, upstream, row),
			})
		}
		out = append(out, dao.LightwellPackageInput{Name: key.artifact, Group: key.group, Versions: versions})
	}
	return out
}

func mapPythonPackageInputs(resp tangy.PythonPackageDetailListResponse) []dao.LightwellPackageInput {
	out := make([]dao.LightwellPackageInput, 0, len(resp.Results))
	for _, item := range resp.Results {
		versions := make([]dao.LightwellPackageVersionInput, 0, len(item.Versions))
		for _, v := range item.Versions {
			license := v.LicenseExpression
			if license == "" {
				license = v.License
			}
			versions = append(versions, dao.LightwellPackageVersionInput{
				Version:     v.Version,
				PublishedAt: v.LastUpdated,
				Purl:        coords.BuildPURL(config.ContentTypePython, "", item.NameNormalized, v.Version),
				Details: &dao.LightwellPackageVersionDetailsInput{
					ProjectURL:  v.ProjectURL,
					License:     license,
					Summary:     v.Summary,
					Description: v.Description,
					Author:      v.Author,
					AuthorEmail: v.AuthorEmail,
				},
			})
		}
		out = append(out, dao.LightwellPackageInput{Name: item.NameNormalized, Group: "", Versions: versions})
	}
	return out
}

func mapNpmPackageInputs(resp tangy.NpmPackageListResponse) []dao.LightwellPackageInput {
	out := make([]dao.LightwellPackageInput, 0, len(resp.Results))
	for _, item := range resp.Results {
		scope, name := handler.ParseNpmPackageName(item.Name)
		created := make(map[string]string, len(item.LatestVersions))
		for _, v := range item.LatestVersions {
			created[v.Version] = v.CreatedAt
		}
		versions := make([]dao.LightwellPackageVersionInput, 0, len(item.Versions))
		for _, v := range item.Versions {
			versions = append(versions, dao.LightwellPackageVersionInput{
				Version:     v,
				PublishedAt: created[v],
				Purl:        coords.BuildPURL(config.ContentTypeNpm, scope, name, v),
			})
		}
		out = append(out, dao.LightwellPackageInput{Name: name, Group: scope, Versions: versions})
	}
	return out
}
