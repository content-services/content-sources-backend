package catalog

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/coverage/matcher"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/external_repos"
	"github.com/content-services/content-sources-backend/pkg/lightwell/rhlw"
	"github.com/content-services/tang/pkg/tangy"
	"github.com/rs/zerolog"
)

const (
	javascriptValidatedBasePath = "javascript/validated"
	javaValidatedBasePath       = "java/validated"
	javaRemediatedBasePath      = "java/remediated"
	pythonValidatedBasePath     = "python/validated"
	pythonRemediatedBasePath    = "python/remediated"
)

// LoadCatalog returns packages from the validated and remediated catalog and the time the catalog was fetched
func LoadCatalog(ctx context.Context, daoReg *dao.DaoRegistry, pulp pulp_client.PulpClient, tang tangy.Tangy, source string) ([]matcher.Package, time.Time, error) {
	snapshotAt := time.Now().UTC()

	if source == config.CatalogSourceDB {
		return loadCatalogFromDB(ctx, daoReg, snapshotAt)
	}
	return loadCatalogFromPulp(ctx, daoReg, pulp, tang, snapshotAt)
}

const mavenCatalogPageSize = 500

func listPackages(ctx context.Context, pulp pulp_client.PulpClient, tang tangy.Tangy, href string, contentType string) ([]matcher.Package, error) {
	switch contentType {
	case config.ContentTypeMaven:
		return listMavenPackages(ctx, pulp, href)
	case config.ContentTypePython:
		return listPythonPackages(ctx, tang, href)
	case config.ContentTypeNpm:
		return listNpmPackages(ctx, tang, href)
	default:
		return nil, fmt.Errorf("unsupported content type %q", contentType)
	}
}

func listMavenPackages(ctx context.Context, pulp pulp_client.PulpClient, href string) ([]matcher.Package, error) {
	var catalog []matcher.Package
	for offset := 0; ; offset += mavenCatalogPageSize {
		resp, err := pulp.ListMavenPackages(ctx, href, "", mavenCatalogPageSize, offset)
		if err != nil {
			return nil, fmt.Errorf("failed to list maven packages: %w", err)
		}
		for _, item := range resp.Results {
			for _, version := range item.Versions {
				upstreamVersion, _ := rhlw.SplitVersion(version)
				catalog = append(catalog, matcher.Package{
					Ecosystem: matcher.EcosystemJava,
					Namespace: item.GroupId,
					Name:      item.ArtifactId,
					Version:   upstreamVersion,
				})
			}
		}
		if len(resp.Results) == 0 || offset+len(resp.Results) >= int(resp.Count) {
			break
		}
	}
	return catalog, nil
}

func listPythonPackages(ctx context.Context, tang tangy.Tangy, href string) ([]matcher.Package, error) {
	var catalog []matcher.Package
	pageCount := 0
	for offset := 0; ; offset += tangy.DefaultLimit {
		resp, err := tang.PythonPackageList(ctx, href, tangy.PythonPackageListFilters{}, tangy.PageOptions{Offset: offset, Limit: tangy.DefaultLimit})
		if err != nil {
			return nil, fmt.Errorf("failed to list python packages: %w", err)
		}
		pageCount++
		for _, item := range resp.Results {
			for _, version := range item.Versions {
				upstreamVersion, _ := rhlw.SplitVersion(version)
				catalog = append(catalog, matcher.Package{
					Ecosystem: matcher.EcosystemPython,
					Name:      item.NameNormalized,
					Version:   upstreamVersion,
				})
			}
		}
		if len(resp.Results) == 0 || offset+len(resp.Results) >= resp.Total {
			break
		}
	}
	return catalog, nil
}

func loadCatalogFromDB(ctx context.Context, daoReg *dao.DaoRegistry, snapshotAt time.Time) ([]matcher.Package, time.Time, error) {
	logger := zerolog.Ctx(ctx)
	start := time.Now()

	rows, err := daoReg.LightwellPackage.InternalOnlyListLensCatalog(ctx)
	if err != nil {
		return nil, time.Time{}, err
	}

	catalog := make([]matcher.Package, 0, len(rows))
	for _, row := range rows {
		version := row.UpstreamVersion
		if version == "" {
			version, _ = rhlw.SplitVersion(row.Version)
		}
		pkg := matcher.Package{
			Name:      row.Name,
			Namespace: row.Group,
			Version:   version,
		}
		switch row.Ecosystem {
		case config.ContentTypeMaven:
			pkg.Ecosystem = matcher.EcosystemJava
		case config.ContentTypePython:
			pkg.Ecosystem = matcher.EcosystemPython
		case config.ContentTypeNpm:
			pkg.Ecosystem = matcher.EcosystemJavaScript
			if pkg.Namespace == "-" {
				pkg.Namespace = ""
			}
		default:
			continue
		}
		catalog = append(catalog, pkg)
	}

	if len(catalog) == 0 {
		return nil, time.Time{}, fmt.Errorf("no validated or remediated catalog packages found")
	}

	logger.Debug().
		Int("packages", len(catalog)).
		Float64("duration_seconds", time.Since(start).Seconds()).
		Str("catalog_source", config.CatalogSourceDB).
		Msg("loaded catalog packages")

	return catalog, snapshotAt, nil
}

func loadCatalogFromPulp(ctx context.Context, daoReg *dao.DaoRegistry, pulp pulp_client.PulpClient, tang tangy.Tangy, snapshotAt time.Time) ([]matcher.Package, time.Time, error) {
	logger := zerolog.Ctx(ctx)

	domain, err := daoReg.Domain.FetchOrCreateDomain(ctx, config.LightwellOrg)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("failed to fetch domain: %w", err)
	}
	pulp = pulp.WithDomain(domain)

	repos, err := daoReg.RepositoryConfig.InternalOnly_FetchRepoConfigForOrg(ctx, config.LightwellOrg)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("failed to fetch repoConfig: %w", err)
	}

	catalogRepos := make([]api.RepositoryResponse, 0, 5)
	for _, repo := range repos {
		if slices.Contains(config.Get().Options.LightwellIgnoreRepos, repo.Name) {
			continue
		}
		if isCatalogBasePath(repo.PublishedDistBasePath) {
			catalogRepos = append(catalogRepos, repo)
		}
	}
	if len(catalogRepos) == 0 {
		return nil, time.Time{}, fmt.Errorf("no validated or remediated repositories found")
	}

	catalog := make([]matcher.Package, 0)
	for _, repo := range catalogRepos {
		start := time.Now()
		href, err := pulp.ResolveRepositoryFromBasePath(ctx, repo.PublishedDistBasePath)
		if err != nil {
			return nil, time.Time{}, fmt.Errorf("failed to resolve repo %s: %w", repo.PublishedDistBasePath, err)
		}
		if href == nil {
			return nil, time.Time{}, fmt.Errorf("failed to resolve repo %s", repo.PublishedDistBasePath)
		}

		pkgs, err := listPackages(ctx, pulp, tang, *href, repo.ContentType)
		if err != nil {
			return nil, time.Time{}, err
		}
		logger.Debug().
			Str("repo", repo.PublishedDistBasePath).
			Int("packages", len(pkgs)).
			Float64("duration_seconds", time.Since(start).Seconds()).
			Str("catalog_source", config.CatalogSourcePulp).
			Msg("loaded catalog packages")
		catalog = append(catalog, pkgs...)
	}

	return catalog, snapshotAt, nil
}

func listNpmPackages(ctx context.Context, tang tangy.Tangy, href string) ([]matcher.Package, error) {
	var catalog []matcher.Package
	for offset := 0; ; offset += tangy.DefaultLimit {
		resp, err := tang.NpmPackageList(ctx, href, tangy.NpmPackageListFilters{}, tangy.PageOptions{Offset: offset, Limit: tangy.DefaultLimit})
		if err != nil {
			return nil, fmt.Errorf("failed to list npm packages: %w", err)
		}
		for _, item := range resp.Results {
			namespace, name := "", item.Name
			if strings.HasPrefix(name, "@") {
				namespace, name, _ = strings.Cut(name, "/")
			}
			for _, version := range item.Versions {
				upstreamVersion, _ := rhlw.SplitVersion(version)
				catalog = append(catalog, matcher.Package{Ecosystem: matcher.EcosystemJavaScript, Namespace: namespace, Name: name, Version: upstreamVersion})
			}
		}
		if len(resp.Results) == 0 || offset+len(resp.Results) >= resp.Total {
			break
		}
	}
	return catalog, nil
}

// EnabledEcosystems returns ecosystems with at least one catalog repository enabled
// by operator configuration. Package counts do not determine ecosystem support.
func EnabledEcosystems() ([]string, error) {
	entries, err := external_repos.LoadLightwellAllowlist()
	if err != nil {
		return nil, err
	}
	ecosystems := make([]string, 0, 3)
	for _, entry := range entries {
		if !isCatalogBasePath(entry.BasePath) || slices.Contains(config.Get().Options.LightwellIgnoreRepos, entry.Name) {
			continue
		}
		var ecosystem string
		switch entry.Type {
		case config.ContentTypeMaven:
			ecosystem = matcher.EcosystemJava
		case config.ContentTypePython:
			ecosystem = matcher.EcosystemPython
		case config.ContentTypeNpm:
			ecosystem = matcher.EcosystemJavaScript
		default:
			continue
		}
		if !slices.Contains(ecosystems, ecosystem) {
			ecosystems = append(ecosystems, ecosystem)
		}
	}
	return ecosystems, nil
}

func isCatalogBasePath(basePath string) bool {
	switch basePath {
	case javaValidatedBasePath, javaRemediatedBasePath, pythonValidatedBasePath, pythonRemediatedBasePath, javascriptValidatedBasePath:
		return true
	default:
		return false
	}
}
