package catalog

import (
	"context"
	"fmt"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/coverage/matcher"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/tang/pkg/tangy"
	"github.com/rs/zerolog"
)

const (
	javaValidatedBasePath    = "java/validated"
	javaRemediatedBasePath   = "java/remediated"
	pythonValidatedBasePath  = "python/validated"
	pythonRemediatedBasePath = "python/remediated"
)

// LoadCatalog returns packages from the validated and remediated catalog and the time the catalog was fetched
func LoadCatalog(ctx context.Context, daoReg *dao.DaoRegistry, pulp pulp_client.PulpClient, tang tangy.Tangy) ([]matcher.Package, time.Time, error) {
	logger := zerolog.Ctx(ctx)
	snapshotAt := time.Now().UTC()

	domain, err := daoReg.Domain.FetchOrCreateDomain(ctx, config.LightwellOrg)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("failed to fetch domain: %w", err)
	}
	pulp = pulp.WithDomain(domain)

	repos, err := daoReg.RepositoryConfig.InternalOnly_FetchRepoConfigForOrg(ctx, config.LightwellOrg)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("failed to fetch repoConfig: %w", err)
	}

	catalogRepos := make([]api.RepositoryResponse, 0, 4)
	for _, repo := range repos {
		if repo.PublishedDistBasePath == javaValidatedBasePath ||
			repo.PublishedDistBasePath == javaRemediatedBasePath ||
			repo.PublishedDistBasePath == pythonValidatedBasePath ||
			repo.PublishedDistBasePath == pythonRemediatedBasePath {
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

		pkgs, err := listPackages(ctx, tang, *href, repo.ContentType)
		if err != nil {
			return nil, time.Time{}, err
		}
		logger.Debug().
			Str("repo", repo.PublishedDistBasePath).
			Int("packages", len(pkgs)).
			Float64("duration_seconds", time.Since(start).Seconds()).
			Msg("loaded catalog packages")
		catalog = append(catalog, pkgs...)
	}
	return catalog, snapshotAt, nil
}

func listPackages(ctx context.Context, tang tangy.Tangy, href string, contentType string) ([]matcher.Package, error) {
	switch contentType {
	case config.ContentTypeMaven:
		return listMavenPackages(ctx, tang, href)
	case config.ContentTypePython:
		return listPythonPackages(ctx, tang, href)
	default:
		return nil, fmt.Errorf("unsupported content type %q", contentType)
	}
}

func listMavenPackages(ctx context.Context, tang tangy.Tangy, href string) ([]matcher.Package, error) {
	var catalog []matcher.Package
	pageCount := 0
	for offset := 0; ; offset += tangy.DefaultLimit {
		resp, err := tang.MavenPackageList(ctx, href, tangy.MavenPackageListFilters{}, tangy.PageOptions{Offset: offset, Limit: tangy.DefaultLimit})
		if err != nil {
			return nil, fmt.Errorf("failed to list maven packages: %w", err)
		}
		pageCount++
		for _, item := range resp.Results {
			for _, version := range item.Versions {
				catalog = append(catalog, matcher.Package{
					Ecosystem: matcher.EcosystemJava,
					Namespace: item.GroupID,
					Name:      item.ArtifactID,
					Version:   version,
				})
			}
		}
		if len(resp.Results) == 0 || offset+len(resp.Results) >= resp.Total {
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
				catalog = append(catalog, matcher.Package{
					Ecosystem: matcher.EcosystemPython,
					Name:      item.NameNormalized,
					Version:   version,
				})
			}
		}
		if len(resp.Results) == 0 || offset+len(resp.Results) >= resp.Total {
			break
		}
	}
	return catalog, nil
}
