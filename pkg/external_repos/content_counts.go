package external_repos

import (
	"context"
	"errors"
	"fmt"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/cache"
	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/tang/pkg/tangy"
	"github.com/rs/zerolog/log"
)

// UpdateContentCounts fetches content counts from pulp and updates the database for all repositories in the given organization
func UpdateContentCounts(ctx context.Context, registry *dao.DaoRegistry, pulpClient pulp_client.PulpClient, tang tangy.Tangy, domainName string) error {
	return UpdateContentCountsWithCache(ctx, registry, pulpClient, tang, cache.Initialize(), domainName)
}

// UpdateContentCountsWithCache is like UpdateContentCounts but allows injecting a custom cache for testing
func UpdateContentCountsWithCache(ctx context.Context, registry *dao.DaoRegistry, pulpClient pulp_client.PulpClient, tang tangy.Tangy, c cache.Cache, domainName string) error {
	repos, err := registry.RepositoryConfig.InternalOnly_FetchRepoConfigForOrg(ctx, config.LightwellOrg)
	if err != nil {
		return fmt.Errorf("failed to fetch repoConfig: %w", err)
	}

	for _, repo := range repos {
		basePath := repo.PublishedDistBasePath
		repoHref, err := pulpClient.ResolveRepositoryFromBasePath(ctx, basePath)
		if err != nil || repoHref == nil {
			log.Error().Err(err).Msgf("Failed to resolve repo %s", repo.Name)
			continue
		}

		pkgCount, buildCount, updated, err := GetContentCountsWithCache(ctx, pulpClient, tang, c, domainName, repo)
		if err != nil {
			log.Error().Err(err).Msgf("Failed to get content counts for repo %s", repo.Name)
			continue
		}
		if updated && (pkgCount != repo.PackageCount || buildCount != repo.BuildCount) {
			err = registry.Repository.InternalOnly_UpdateCounts(ctx, repo.RepositoryUUID, pkgCount, buildCount)
			if err != nil {
				log.Error().Err(err).Msg("Failed to update repository counts")
			}
		}
	}
	return nil
}

// GetContentCounts retrieves content counts from cache or pulp for a given repository
func GetContentCounts(ctx context.Context, pulpClient pulp_client.PulpClient, tang tangy.Tangy, domainName string, repo api.RepositoryResponse) (pkgCount int, buildCount int, updated bool, err error) {
	return GetContentCountsWithCache(ctx, pulpClient, tang, cache.Initialize(), domainName, repo)
}

// GetContentCountsWithCache is like GetContentCounts but allows injecting a custom cache for testing
func GetContentCountsWithCache(ctx context.Context, pulpClient pulp_client.PulpClient, tang tangy.Tangy, c cache.Cache, domainName string, repo api.RepositoryResponse) (pkgCount int, buildCount int, updated bool, err error) {
	cachedCounts, err := c.GetContentCounts(ctx, domainName, repo.UUID)
	if err != nil && !errors.Is(err, cache.ErrNotFound) {
		log.Error().Err(err).Msg("Content counts - error reading from cache")
	}
	if cachedCounts != nil {
		return cachedCounts.Packages, cachedCounts.Builds, updated, nil
	}

	repoHref, err := pulpClient.ResolveRepositoryFromBasePath(ctx, repo.PublishedDistBasePath)
	if err != nil {
		return 0, 0, updated, err
	}
	if repoHref == nil {
		return 0, 0, updated, fmt.Errorf("failed to resolve repo %s", repo.Name)
	}

	pkgCount, buildCount, err = ContentCountsForType(ctx, tang, *repoHref, repo.ContentType)
	if err != nil {
		return 0, 0, updated, err
	}

	err = c.SetContentCounts(ctx, domainName, repo.UUID, cache.RepoContentCount{
		Packages: pkgCount,
		Builds:   buildCount,
	})
	if err != nil {
		return 0, 0, updated, fmt.Errorf("failed to cache content counts for repo %s: %w", repo.Name, err)
	}
	return pkgCount, buildCount, true, nil
}

// ContentCountsForType retrieves package and build counts for a repository based on its content type
func ContentCountsForType(ctx context.Context, tang tangy.Tangy, repoHref string, contentType string) (int, int, error) {
	switch contentType {
	case config.ContentTypePython:
		pkgResp, err := tang.PythonPackageList(ctx, repoHref, tangy.PythonPackageListFilters{}, tangy.PageOptions{Limit: 1})
		if err != nil {
			return 0, 0, fmt.Errorf("failed to get Python package list: %w", err)
		}
		return pkgResp.Total, pkgResp.Total, nil
	case config.ContentTypeMaven:
		pkgResp, err := tang.MavenPackageList(ctx, repoHref, tangy.MavenPackageListFilters{}, tangy.PageOptions{Limit: 1})
		if err != nil {
			return 0, 0, fmt.Errorf("failed to fetch Maven package list: %w", err)
		}
		buildResp, err := tang.MavenBuildList(ctx, repoHref, "", "", "", tangy.PageOptions{Limit: 1})
		if err != nil {
			return 0, 0, fmt.Errorf("failed to fetch build list for repo %s: %w", repoHref, err)
		}
		return pkgResp.Total, buildResp.Total, nil
	default:
		return 0, 0, fmt.Errorf("unknown content type: %s", contentType)
	}
}
