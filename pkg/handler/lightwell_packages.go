package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/rbac"
	"github.com/content-services/tang/pkg/tangy"
	zest "github.com/content-services/zest/release/v2026"
	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog/log"
)

type LightwellPackagesHandler struct {
	DaoRegistry dao.DaoRegistry
	TangClient  tangy.Tangy
	PulpClient  pulp_client.PulpClient
}

func RegisterLightwellPackageRoutes(engine *echo.Group, daoReg *dao.DaoRegistry, tangClient tangy.Tangy, pulpClient pulp_client.PulpClient) {
	h := LightwellPackagesHandler{
		DaoRegistry: *daoReg,
		TangClient:  tangClient,
		PulpClient:  pulpClient,
	}
	addRepoRoute(engine, http.MethodGet, "/lightwell/packages", h.ListPackages, rbac.RbacVerbRead)
	addRepoRoute(engine, http.MethodGet, "/lightwell/package_versions", h.ListPackageVersions, rbac.RbacVerbRead)
}

// listLightwellPackages godoc
// @Summary      List Lightwell Packages (cross-repo)
// @ID           listLightwellPackages
// @Description  List packages aggregated across all Lightwell repositories, with optional filtering by ecosystem, name, and security level.
// @Tags         lightwell
// @Accept       json
// @Produce      json
// @Param        ecosystem       query  string  false  "Filter by ecosystem (maven, python, npm)"
// @Param        name            query  string  false  "Filter by package name (substring match)"
// @Param        security_level  query  string  false  "Filter by security level (validated, remediated)"
// @Param        limit           query  int     false  "Limit of results to return"
// @Param        offset          query  int     false  "Offset into results"
// @Success      200 {object} api.LightwellPackageCollectionResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /lightwell/packages [get]
func (h *LightwellPackagesHandler) ListPackages(c echo.Context) error {
	page := ParsePagination(c)
	filters := parseLightwellPackageFilters(c)

	if err := validateContentType(filters.Ecosystem); err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Invalid ecosystem filter", err.Error())
	}

	repos, err := h.fetchLightwellRepos(c, filters.Ecosystem, filters.SecurityLevel)
	if err != nil {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Error listing Lightwell repositories", err.Error())
	}
	if filters.Repository != "" {
		repos = filterReposByName(repos, filters.Repository)
	}

	// Fast path: when the filters resolve to a single repository there is no
	// cross-repo merge to do, so push pagination down to Tang and take the count
	// from its total instead of fetching every package.
	if len(repos) == 1 {
		paged, totalCount, err := h.fetchPackagesPage(c.Request().Context(), repos[0], filters.Name, page.SortBy, page.Offset, page.Limit)
		if err != nil {
			return ce.NewErrorResponse(http.StatusInternalServerError, "Error retrieving packages", err.Error())
		}
		resp := api.LightwellPackageCollectionResponse{Data: paged}
		collResp := SetCollectionResponseMetadata(&resp, c, totalCount)
		return c.JSON(http.StatusOK, collResp)
	}

	items, err := h.aggregatePackages(c.Request().Context(), repos, filters.Name)
	if err != nil {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Error retrieving packages", err.Error())
	}

	sortLightwellPackages(items, page.SortBy)
	totalCount := int64(len(items))
	paged := paginatePackages(items, page.Offset, page.Limit)
	resp := api.LightwellPackageCollectionResponse{Data: paged}
	collResp := SetCollectionResponseMetadata(&resp, c, totalCount)
	return c.JSON(http.StatusOK, collResp)
}

// tangSortBy translates the handler's sort_by ("<field> [asc|desc]") into the
// sort string passed to Tang via PageOptions.SortBy. It returns "" for the
// default sort so Tang uses its natural ordering. Tang does not yet honor this
// field for package lists (see tangSupportsPackageSort); forwarding it here means
// the fast path works unchanged once tang implements server-side sorting.
func tangSortBy(sortBy string) string {
	if sortBy == "" {
		return ""
	}
	field, dir := parseSortBy(sortBy)
	if field == "" {
		field = "name"
	}
	return field + " " + dir
}

// fetchPackagesPage fetches a single page of packages for one repo directly from
// Tang, returning the mapped items and the repo's total package count.
func (h *LightwellPackagesHandler) fetchPackagesPage(ctx context.Context, repo api.RepositoryResponse, nameSearch, sortBy string, offset, limit int) ([]api.LightwellPackageResponse, int64, error) {
	if repo.PublishedDistBasePath == "" {
		return []api.LightwellPackageResponse{}, 0, nil
	}

	pulpClient, repositoryHref, err := h.resolveRepository(ctx, repo)
	if err != nil {
		return nil, 0, err
	}

	pageOpts := tangy.PageOptions{Offset: offset, Limit: limit, SortBy: tangSortBy(sortBy)}

	switch repo.ContentType {
	case config.ContentTypeMaven:
		pulpResp, err := pulpClient.ListMavenPackages(ctx, repositoryHref, nameSearch, limit, offset)
		if err != nil {
			return nil, 0, err
		}
		return mapMavenToLightwellPackages(pulpResp, repo), int64(pulpResp.Count), nil

	case config.ContentTypePython:
		tangResp, err := h.TangClient.PythonPackageList(ctx, repositoryHref, tangy.PythonPackageListFilters{Search: nameSearch}, pageOpts)
		if err != nil {
			return nil, 0, err
		}
		return mapPythonToLightwellPackages(tangResp, repo), int64(tangResp.Total), nil

	case config.ContentTypeNpm:
		tangResp, err := h.TangClient.NpmPackageList(ctx, repositoryHref, tangy.NpmPackageListFilters{Search: nameSearch}, pageOpts)
		if err != nil {
			return nil, 0, err
		}
		return mapNpmToLightwellPackages(tangResp, repo), int64(tangResp.Total), nil

	default:
		return []api.LightwellPackageResponse{}, 0, nil
	}
}

// listLightwellPackageVersions godoc
// @Summary      List Lightwell Package Versions (cross-repo)
// @ID           listLightwellPackageVersions
// @Description  List individual package versions aggregated across all Lightwell repositories, with optional CVE-based filtering.
// @Tags         lightwell
// @Accept       json
// @Produce      json
// @Param        ecosystem            query  string  false  "Filter by ecosystem (maven, python, npm)"
// @Param        name                 query  string  false  "Filter by package name (substring match)"
// @Param        security_level       query  string  false  "Filter by security level (validated, remediated)"
// @Param        repository           query  string  false  "Filter by repository name"
// @Param        resolves_cve_id      query  string  false  "Show only packages that resolve this CVE"
// @Param        vulnerable_to_cve_id query  string  false  "Show only packages vulnerable to this CVE"
// @Param        limit                query  int     false  "Limit of results to return"
// @Param        offset               query  int     false  "Offset into results"
// @Success      200 {object} api.LightwellPackageVersionCollectionResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /lightwell/package_versions [get]
func (h *LightwellPackagesHandler) ListPackageVersions(c echo.Context) error {
	page := ParsePagination(c)
	filters := parseLightwellPackageVersionFilters(c)

	if err := validateContentType(filters.Ecosystem); err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Invalid ecosystem filter", err.Error())
	}

	repos, err := h.fetchLightwellRepos(c, filters.Ecosystem, filters.SecurityLevel)
	if err != nil {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Error listing Lightwell repositories", err.Error())
	}
	if filters.Repository != "" {
		repos = filterReposByName(repos, filters.Repository)
	}

	items, err := h.aggregatePackageVersions(c.Request().Context(), repos, filters.Name)
	if err != nil {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Error retrieving package versions", err.Error())
	}

	if filters.ResolvesCveID != "" {
		items, err = h.filterVersionsByResolvingCve(c.Request().Context(), items, filters.ResolvesCveID)
		if err != nil {
			return ce.NewErrorResponse(http.StatusInternalServerError, "Error filtering by CVE", err.Error())
		}
	}
	if filters.VulnerableToCveID != "" {
		items, err = h.filterVersionsByVulnerableCve(c.Request().Context(), items, filters.VulnerableToCveID)
		if err != nil {
			return ce.NewErrorResponse(http.StatusInternalServerError, "Error filtering by CVE", err.Error())
		}
	}

	sortLightwellVersions(items, page.SortBy)
	totalCount := int64(len(items))
	paged := paginateVersions(items, page.Offset, page.Limit)
	resp := api.LightwellPackageVersionCollectionResponse{Data: paged}
	collResp := SetCollectionResponseMetadata(&resp, c, totalCount)
	return c.JSON(http.StatusOK, collResp)
}

// fetchLightwellRepos returns Lightwell repos for the caller's org, optionally
// filtered by content type and security level.
func (h *LightwellPackagesHandler) fetchLightwellRepos(c echo.Context, contentType, securityLevel string) ([]api.RepositoryResponse, error) {
	_, orgID := GetAccountIdOrgId(c)
	ctx := c.Request().Context()

	filter := api.FilterData{Origin: config.OriginLightwell}
	if contentType != "" {
		filter.ContentType = contentType
	}

	repos, _, err := h.DaoRegistry.RepositoryConfig.List(ctx, orgID, api.PaginationData{Limit: MaxLimit}, filter)
	if err != nil {
		return nil, err
	}

	if securityLevel == "" {
		return repos.Data, nil
	}
	filtered := make([]api.RepositoryResponse, 0, len(repos.Data))
	for _, r := range repos.Data {
		if strings.EqualFold(r.SecurityLevel, securityLevel) {
			filtered = append(filtered, r)
		}
	}
	return filtered, nil
}

type repoPackageResult struct {
	repo api.RepositoryResponse
	pkgs []api.LightwellPackageResponse
	err  error
}

// aggregatePackages queries Tang for each repo in parallel and merges results.
func (h *LightwellPackagesHandler) aggregatePackages(ctx context.Context, repos []api.RepositoryResponse, nameSearch string) ([]api.LightwellPackageResponse, error) {
	results := make([]repoPackageResult, len(repos))
	var wg sync.WaitGroup

	for i, repo := range repos {
		wg.Add(1)
		go func(idx int, r api.RepositoryResponse) {
			defer wg.Done()
			pkgs, err := h.fetchPackagesFromRepo(ctx, r, nameSearch)
			results[idx] = repoPackageResult{repo: r, pkgs: pkgs, err: err}
		}(i, repo)
	}
	wg.Wait()

	var combined []api.LightwellPackageResponse
	var errs []error
	for _, res := range results {
		if res.err != nil {
			errs = append(errs, fmt.Errorf("repo %s: %w", res.repo.Name, res.err))
			continue
		}
		combined = append(combined, res.pkgs...)
	}

	if len(errs) > 0 && len(combined) == 0 {
		return nil, errors.Join(errs...)
	}
	if len(errs) > 0 {
		log.Warn().Errs("errors", errs).Msg("partial failure fetching cross-repo packages")
	}

	return combined, nil
}

// fetchAllPages repeatedly calls fetchPage with an increasing offset until every
// package reported by Tang has been retrieved, merging the mapped items. This
// avoids silently truncating repos that hold more than MaxLimit packages.
//
// fetchPage returns the mapped items for the page, the number of raw packages in
// the page (used to advance the offset — this may differ from len(items) when a
// package expands into multiple items), and the total package count reported by
// Tang.
func fetchAllPages[T any](fetchPage func(offset, limit int) (items []T, pageCount, total int, err error)) ([]T, error) {
	var all []T
	offset := 0
	for {
		items, pageCount, total, err := fetchPage(offset, MaxLimit)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
		offset += pageCount
		if pageCount == 0 || offset >= total {
			break
		}
	}
	return all, nil
}

func (h *LightwellPackagesHandler) fetchPackagesFromRepo(ctx context.Context, repo api.RepositoryResponse, nameSearch string) ([]api.LightwellPackageResponse, error) {
	if repo.PublishedDistBasePath == "" {
		return nil, nil
	}

	pulpClient, repositoryHref, err := h.resolveRepository(ctx, repo)
	if err != nil {
		return nil, err
	}

	switch repo.ContentType {
	case config.ContentTypeMaven:
		return fetchAllPages(func(offset, limit int) ([]api.LightwellPackageResponse, int, int, error) {
			pulpResp, err := pulpClient.ListMavenPackages(ctx, repositoryHref, nameSearch, limit, offset)
			if err != nil {
				return nil, 0, 0, err
			}
			return mapMavenToLightwellPackages(pulpResp, repo), len(pulpResp.Results), int(pulpResp.Count), nil
		})

	case config.ContentTypePython:
		return fetchAllPages(func(offset, limit int) ([]api.LightwellPackageResponse, int, int, error) {
			tangResp, err := h.TangClient.PythonPackageList(ctx, repositoryHref, tangy.PythonPackageListFilters{Search: nameSearch}, tangy.PageOptions{Offset: offset, Limit: limit})
			if err != nil {
				return nil, 0, 0, err
			}
			return mapPythonToLightwellPackages(tangResp, repo), len(tangResp.Results), tangResp.Total, nil
		})

	case config.ContentTypeNpm:
		return fetchAllPages(func(offset, limit int) ([]api.LightwellPackageResponse, int, int, error) {
			tangResp, err := h.TangClient.NpmPackageList(ctx, repositoryHref, tangy.NpmPackageListFilters{Search: nameSearch}, tangy.PageOptions{Offset: offset, Limit: limit})
			if err != nil {
				return nil, 0, 0, err
			}
			return mapNpmToLightwellPackages(tangResp, repo), len(tangResp.Results), tangResp.Total, nil
		})

	default:
		return nil, nil
	}
}

type repoVersionResult struct {
	repo     api.RepositoryResponse
	versions []api.LightwellPackageVersionResponse
	err      error
}

// aggregatePackageVersions queries Tang for each repo in parallel and expands
// every package into individual version items.
func (h *LightwellPackagesHandler) aggregatePackageVersions(ctx context.Context, repos []api.RepositoryResponse, nameSearch string) ([]api.LightwellPackageVersionResponse, error) {
	results := make([]repoVersionResult, len(repos))
	var wg sync.WaitGroup

	for i, repo := range repos {
		wg.Add(1)
		go func(idx int, r api.RepositoryResponse) {
			defer wg.Done()
			versions, err := h.fetchVersionsFromRepo(ctx, r, nameSearch)
			results[idx] = repoVersionResult{repo: r, versions: versions, err: err}
		}(i, repo)
	}
	wg.Wait()

	var combined []api.LightwellPackageVersionResponse
	var errs []error
	for _, res := range results {
		if res.err != nil {
			errs = append(errs, fmt.Errorf("repo %s: %w", res.repo.Name, res.err))
			continue
		}
		combined = append(combined, res.versions...)
	}

	if len(errs) > 0 && len(combined) == 0 {
		return nil, errors.Join(errs...)
	}
	if len(errs) > 0 {
		log.Warn().Errs("errors", errs).Msg("partial failure fetching cross-repo versions")
	}

	return combined, nil
}

func (h *LightwellPackagesHandler) fetchVersionsFromRepo(ctx context.Context, repo api.RepositoryResponse, nameSearch string) ([]api.LightwellPackageVersionResponse, error) {
	if repo.PublishedDistBasePath == "" {
		return nil, nil
	}

	pulpClient, repositoryHref, err := h.resolveRepository(ctx, repo)
	if err != nil {
		return nil, err
	}

	switch repo.ContentType {
	case config.ContentTypeMaven:
		return fetchAllPages(func(offset, limit int) ([]api.LightwellPackageVersionResponse, int, int, error) {
			pulpResp, err := pulpClient.ListMavenPackages(ctx, repositoryHref, nameSearch, limit, offset)
			if err != nil {
				return nil, 0, 0, err
			}
			return expandMavenVersions(pulpResp, repo), len(pulpResp.Results), int(pulpResp.Count), nil
		})

	case config.ContentTypePython:
		return fetchAllPages(func(offset, limit int) ([]api.LightwellPackageVersionResponse, int, int, error) {
			tangResp, err := h.TangClient.PythonPackageList(ctx, repositoryHref, tangy.PythonPackageListFilters{Search: nameSearch}, tangy.PageOptions{Offset: offset, Limit: limit})
			if err != nil {
				return nil, 0, 0, err
			}
			return expandPythonVersions(tangResp, repo), len(tangResp.Results), tangResp.Total, nil
		})

	case config.ContentTypeNpm:
		return fetchAllPages(func(offset, limit int) ([]api.LightwellPackageVersionResponse, int, int, error) {
			tangResp, err := h.TangClient.NpmPackageList(ctx, repositoryHref, tangy.NpmPackageListFilters{Search: nameSearch}, tangy.PageOptions{Offset: offset, Limit: limit})
			if err != nil {
				return nil, 0, 0, err
			}
			return expandNpmVersions(tangResp, repo), len(tangResp.Results), tangResp.Total, nil
		})

	default:
		return nil, nil
	}
}

func (h *LightwellPackagesHandler) resolveRepository(ctx context.Context, repo api.RepositoryResponse) (pulp_client.PulpClient, string, error) {
	domainName, err := h.DaoRegistry.Domain.FetchOrCreateDomain(ctx, repo.OrgID)
	if err != nil {
		return nil, "", err
	}
	pulpClient := h.PulpClient.WithDomain(domainName)
	href, err := pulpClient.ResolveRepositoryFromBasePath(ctx, repo.PublishedDistBasePath)
	if err != nil {
		return nil, "", fmt.Errorf("repo %s: %w", repo.UUID, err)
	}
	if href == nil {
		return nil, "", fmt.Errorf("repo %s: distribution not found", repo.UUID)
	}
	return pulpClient, *href, nil
}

// filterVersionsByResolvingCve keeps only versions that fix the given CVE.
func (h *LightwellPackagesHandler) filterVersionsByResolvingCve(ctx context.Context, items []api.LightwellPackageVersionResponse, cveID string) ([]api.LightwellPackageVersionResponse, error) {
	matches, err := h.DaoRegistry.LightwellAdvisory.ListAdvisoriesByCveID(ctx, cveID)
	if err != nil {
		return nil, err
	}

	type repoPackage struct{ repo, name string }
	fixedSet := make(map[repoPackage]map[string]bool)
	for _, m := range matches {
		key := repoPackage{repo: m.RepoName, name: m.PackageName}
		if fixedSet[key] == nil {
			fixedSet[key] = make(map[string]bool)
		}
		for _, v := range m.FixedVersions {
			fixedSet[key][v] = true
		}
	}

	var result []api.LightwellPackageVersionResponse
	for _, item := range items {
		key := repoPackage{repo: item.Repository, name: item.Name}
		if versions, ok := fixedSet[key]; ok && versions[item.Version] {
			result = append(result, item)
		}
	}
	return result, nil
}

// filterVersionsByVulnerableCve keeps only versions of packages affected by
// the given CVE that are NOT in the fixed-versions list.
func (h *LightwellPackagesHandler) filterVersionsByVulnerableCve(ctx context.Context, items []api.LightwellPackageVersionResponse, cveID string) ([]api.LightwellPackageVersionResponse, error) {
	matches, err := h.DaoRegistry.LightwellAdvisory.ListAdvisoriesByCveID(ctx, cveID)
	if err != nil {
		return nil, err
	}

	type repoPackage struct{ repo, name string }
	affectedPackages := make(map[repoPackage]bool)
	fixedSet := make(map[repoPackage]map[string]bool)
	for _, m := range matches {
		key := repoPackage{repo: m.RepoName, name: m.PackageName}
		affectedPackages[key] = true
		if fixedSet[key] == nil {
			fixedSet[key] = make(map[string]bool)
		}
		for _, v := range m.FixedVersions {
			fixedSet[key][v] = true
		}
	}

	var result []api.LightwellPackageVersionResponse
	for _, item := range items {
		key := repoPackage{repo: item.Repository, name: item.Name}
		if affectedPackages[key] && !fixedSet[key][item.Version] {
			result = append(result, item)
		}
	}
	return result, nil
}

// --- PURL / coordinate builders ---

func buildPURL(contentType, group, name, version string) string {
	switch contentType {
	case config.ContentTypeMaven:
		return fmt.Sprintf("pkg:maven/%s/%s@%s", group, name, version)
	case config.ContentTypePython:
		return fmt.Sprintf("pkg:pypi/%s@%s", name, version)
	case config.ContentTypeNpm:
		if group == "-" || group == "" {
			return fmt.Sprintf("pkg:npm/%s@%s", name, version)
		}
		scope := strings.TrimPrefix(group, "@")
		return fmt.Sprintf("pkg:npm/%%40%s/%s@%s", scope, name, version)
	default:
		return ""
	}
}

func buildCoordinates(contentType, group, name string) string {
	switch contentType {
	case config.ContentTypeMaven:
		return fmt.Sprintf("%s:%s", group, name)
	case config.ContentTypePython:
		return name
	case config.ContentTypeNpm:
		if group == "-" || group == "" {
			return name
		}
		return fmt.Sprintf("%s/%s", group, name)
	default:
		return ""
	}
}

// --- mapping helpers ---

func mapMavenToLightwellPackages(resp zest.PaginatedMavenRepositoryPackageListResponse, repo api.RepositoryResponse) []api.LightwellPackageResponse {
	out := make([]api.LightwellPackageResponse, 0, len(resp.Results))
	for _, item := range resp.Results {
		out = append(out, api.LightwellPackageResponse{
			Name:           item.ArtifactId,
			Group:          item.GroupId,
			Ecosystem:      config.ContentTypeMaven,
			Repository:     repo.Name,
			RepositoryUUID: repo.UUID,
			Versions:       item.Versions,
			LatestReleases: mapMavenPackageReleases(item.LatestReleases),
		})
	}
	return out
}

func mapPythonToLightwellPackages(resp tangy.PythonPackageListResponse, repo api.RepositoryResponse) []api.LightwellPackageResponse {
	out := make([]api.LightwellPackageResponse, 0, len(resp.Results))
	for _, item := range resp.Results {
		releases := make([]api.ReleaseInfo, len(item.LatestVersions))
		for j, ver := range item.LatestVersions {
			releases[j] = api.ReleaseInfo{Version: ver.Version, CreatedAt: ver.CreatedAt}
		}
		out = append(out, api.LightwellPackageResponse{
			Name:           item.NameNormalized,
			Ecosystem:      config.ContentTypePython,
			Repository:     repo.Name,
			RepositoryUUID: repo.UUID,
			Versions:       item.Versions,
			LatestReleases: releases,
		})
	}
	return out
}

func mapNpmToLightwellPackages(resp tangy.NpmPackageListResponse, repo api.RepositoryResponse) []api.LightwellPackageResponse {
	out := make([]api.LightwellPackageResponse, 0, len(resp.Results))
	for _, item := range resp.Results {
		releases := make([]api.ReleaseInfo, len(item.LatestVersions))
		for j, ver := range item.LatestVersions {
			releases[j] = api.ReleaseInfo{Version: ver.Version, CreatedAt: ver.CreatedAt}
		}
		scope, name := ParseNpmPackageName(item.Name)
		out = append(out, api.LightwellPackageResponse{
			Name:           name,
			Group:          scope,
			Ecosystem:      config.ContentTypeNpm,
			Repository:     repo.Name,
			RepositoryUUID: repo.UUID,
			Versions:       item.Versions,
			LatestReleases: releases,
		})
	}
	return out
}

func expandMavenVersions(resp zest.PaginatedMavenRepositoryPackageListResponse, repo api.RepositoryResponse) []api.LightwellPackageVersionResponse {
	var out []api.LightwellPackageVersionResponse
	for _, item := range resp.Results {
		relMap := mavenLatestReleaseMap(item.LatestReleases)
		for _, v := range item.Versions {
			ver := api.LightwellPackageVersionResponse{
				Name:           item.ArtifactId,
				Group:          item.GroupId,
				Version:        v,
				Ecosystem:      config.ContentTypeMaven,
				Repository:     repo.Name,
				RepositoryUUID: repo.UUID,
				Purl:           buildPURL(config.ContentTypeMaven, item.GroupId, item.ArtifactId, v),
				Coordinates:    buildCoordinates(config.ContentTypeMaven, item.GroupId, item.ArtifactId),
			}
			if rel, ok := relMap[v]; ok {
				ver.Release = rel.Release
				ver.CreatedAt = rel.CreatedAt
			}
			out = append(out, ver)
		}
	}
	return out
}

func expandPythonVersions(resp tangy.PythonPackageListResponse, repo api.RepositoryResponse) []api.LightwellPackageVersionResponse {
	var out []api.LightwellPackageVersionResponse
	for _, item := range resp.Results {
		verMap := latestVersionMap(item.LatestVersions)
		for _, v := range item.Versions {
			ver := api.LightwellPackageVersionResponse{
				Name:           item.NameNormalized,
				Version:        v,
				Ecosystem:      config.ContentTypePython,
				Repository:     repo.Name,
				RepositoryUUID: repo.UUID,
				Purl:           buildPURL(config.ContentTypePython, "", item.NameNormalized, v),
				Coordinates:    buildCoordinates(config.ContentTypePython, "", item.NameNormalized),
			}
			if info, ok := verMap[v]; ok {
				ver.CreatedAt = info.CreatedAt
			}
			out = append(out, ver)
		}
	}
	return out
}

func expandNpmVersions(resp tangy.NpmPackageListResponse, repo api.RepositoryResponse) []api.LightwellPackageVersionResponse {
	var out []api.LightwellPackageVersionResponse
	for _, item := range resp.Results {
		scope, name := ParseNpmPackageName(item.Name)
		verMap := npmVersionMap(item.LatestVersions)
		for _, v := range item.Versions {
			ver := api.LightwellPackageVersionResponse{
				Name:           name,
				Group:          scope,
				Version:        v,
				Ecosystem:      config.ContentTypeNpm,
				Repository:     repo.Name,
				RepositoryUUID: repo.UUID,
				Purl:           buildPURL(config.ContentTypeNpm, scope, name, v),
				Coordinates:    buildCoordinates(config.ContentTypeNpm, scope, name),
			}
			if info, ok := verMap[v]; ok {
				ver.CreatedAt = info.CreatedAt
			}
			out = append(out, ver)
		}
	}
	return out
}

// --- filter / pagination helpers ---

func parseLightwellPackageFilters(c echo.Context) api.LightwellPackageFilterData {
	var f api.LightwellPackageFilterData
	_ = echo.QueryParamsBinder(c).
		String("ecosystem", &f.Ecosystem).
		String("name", &f.Name).
		String("repository", &f.Repository).
		String("security_level", &f.SecurityLevel).
		BindError()
	return f
}

func parseLightwellPackageVersionFilters(c echo.Context) api.LightwellPackageVersionFilterData {
	var f api.LightwellPackageVersionFilterData
	_ = echo.QueryParamsBinder(c).
		String("ecosystem", &f.Ecosystem).
		String("name", &f.Name).
		String("security_level", &f.SecurityLevel).
		String("repository", &f.Repository).
		String("resolves_cve_id", &f.ResolvesCveID).
		String("vulnerable_to_cve_id", &f.VulnerableToCveID).
		BindError()
	return f
}

var validContentTypes = map[string]bool{
	config.ContentTypeMaven:  true,
	config.ContentTypePython: true,
	config.ContentTypeNpm:    true,
}

func validateContentType(ct string) error {
	if ct == "" {
		return nil
	}
	if !validContentTypes[ct] {
		return fmt.Errorf("unsupported type: %s (must be maven, python, or npm)", ct)
	}
	return nil
}

func filterReposByName(repos []api.RepositoryResponse, name string) []api.RepositoryResponse {
	var out []api.RepositoryResponse
	for _, r := range repos {
		if strings.EqualFold(r.Name, name) {
			out = append(out, r)
		}
	}
	return out
}

func paginatePackages(items []api.LightwellPackageResponse, offset, limit int) []api.LightwellPackageResponse {
	if offset >= len(items) {
		return []api.LightwellPackageResponse{}
	}
	end := offset + limit
	if end > len(items) {
		end = len(items)
	}
	return items[offset:end]
}

func paginateVersions(items []api.LightwellPackageVersionResponse, offset, limit int) []api.LightwellPackageVersionResponse {
	if offset >= len(items) {
		return []api.LightwellPackageVersionResponse{}
	}
	end := offset + limit
	if end > len(items) {
		end = len(items)
	}
	return items[offset:end]
}

// release-info lookup helpers for version expansion

type mavenRelInfo struct {
	Release   string
	CreatedAt string
}

func mavenLatestReleaseMap(releases []zest.MavenPackageReleaseResponse) map[string]mavenRelInfo {
	m := make(map[string]mavenRelInfo, len(releases))
	for _, r := range releases {
		m[r.Version] = mavenRelInfo{Release: r.Release, CreatedAt: r.CreatedAt.Format(time.RFC3339)}
	}
	return m
}

type versionCreatedAt struct {
	CreatedAt string
}

func latestVersionMap(versions []tangy.PythonVersionInfo) map[string]versionCreatedAt {
	m := make(map[string]versionCreatedAt, len(versions))
	for _, v := range versions {
		m[v.Version] = versionCreatedAt{CreatedAt: v.CreatedAt}
	}
	return m
}

func npmVersionMap(versions []tangy.NpmVersionInfo) map[string]versionCreatedAt {
	m := make(map[string]versionCreatedAt, len(versions))
	for _, v := range versions {
		m[v.Version] = versionCreatedAt{CreatedAt: v.CreatedAt}
	}
	return m
}

// --- sort helpers ---

func sortLightwellPackages(items []api.LightwellPackageResponse, sortBy string) {
	field, dir := parseSortBy(sortBy)
	if field == "" {
		field = "name"
	}
	sort.SliceStable(items, func(i, j int) bool {
		var less bool
		switch field {
		case "ecosystem":
			less = items[i].Ecosystem < items[j].Ecosystem
		case "repository":
			less = items[i].Repository < items[j].Repository
		default: // "name" (and default): match Tang's (group, name) ordering
			less = lessByGroupName(items[i].Group, items[i].Name, items[j].Group, items[j].Name)
		}
		if dir == "desc" {
			return !less
		}
		return less
	})
}

// lessByGroupName orders by group then name, matching Tang's native ordering
// (Maven: group_id, artifact_id; Python/npm have an empty group so this reduces
// to name ordering).
func lessByGroupName(groupA, nameA, groupB, nameB string) bool {
	if groupA != groupB {
		return groupA < groupB
	}
	return nameA < nameB
}

func sortLightwellVersions(items []api.LightwellPackageVersionResponse, sortBy string) {
	field, dir := parseSortBy(sortBy)
	if field == "" {
		field = "name"
	}
	sort.SliceStable(items, func(i, j int) bool {
		var less bool
		switch field {
		case "version":
			less = items[i].Version < items[j].Version
		case "ecosystem":
			less = items[i].Ecosystem < items[j].Ecosystem
		case "repository":
			less = items[i].Repository < items[j].Repository
		default: // "name" (and default): match Tang's (group, name, version) ordering
			if items[i].Group != items[j].Group {
				less = items[i].Group < items[j].Group
			} else if items[i].Name != items[j].Name {
				less = items[i].Name < items[j].Name
			} else {
				less = items[i].Version < items[j].Version
			}
		}
		if dir == "desc" {
			return !less
		}
		return less
	})
}

func parseSortBy(sortBy string) (field, direction string) {
	if sortBy == "" {
		return "", "asc"
	}
	parts := strings.Fields(sortBy)
	field = strings.ToLower(parts[0])
	direction = "asc"
	if len(parts) > 1 && strings.EqualFold(parts[1], "desc") {
		direction = "desc"
	}
	return field, direction
}
