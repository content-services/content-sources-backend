package commands

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/handler"
	"github.com/content-services/content-sources-backend/pkg/lightwell/coords"
	"github.com/content-services/tang/pkg/tangy"
	zest "github.com/content-services/zest/release/v2026"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v2"
	"gorm.io/gorm"
)

const importMaxConcurrentRepos = 10
const importPageSize = 300

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

	pkgs, err := fetchRepoPackages(ctx, pulpClient, repo.ContentType, *repoHref)
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

func fetchRepoPackages(ctx context.Context, pulpClient pulp_client.PulpClient, contentType, repoHref string) ([]dao.LightwellPackageInput, error) {
	switch contentType {
	case config.ContentTypeMaven:
		var all []dao.LightwellPackageInput
		offset := 0
		for {
			resp, err := pulpClient.ListMavenPackages(ctx, repoHref, "", importPageSize, offset)
			if err != nil {
				return nil, err
			}
			all = append(all, mapMavenPackageInputs(resp)...)
			offset += len(resp.Results)
			if len(resp.Results) == 0 || int64(offset) >= resp.Count {
				break
			}
		}
		return all, nil
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

func mapMavenPackageInputs(resp zest.PaginatedMavenRepositoryPackageListResponse) []dao.LightwellPackageInput {
	out := make([]dao.LightwellPackageInput, 0, len(resp.Results))
	for _, item := range resp.Results {
		rel := make(map[string]zest.MavenPackageReleaseResponse, len(item.LatestReleases))
		for _, r := range item.LatestReleases {
			rel[r.Version] = r
		}
		versions := make([]dao.LightwellPackageVersionInput, 0, len(item.Versions))
		for _, v := range item.Versions {
			in := dao.LightwellPackageVersionInput{
				Version: v,
				Purl:    coords.BuildPURL(config.ContentTypeMaven, item.GroupId, item.ArtifactId, v),
			}
			if r, ok := rel[v]; ok {
				in.Release = r.Release
				in.PublishedAt = r.CreatedAt.Format(time.RFC3339)
			}
			versions = append(versions, in)
		}
		out = append(out, dao.LightwellPackageInput{Name: item.ArtifactId, Group: item.GroupId, Versions: versions})
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
