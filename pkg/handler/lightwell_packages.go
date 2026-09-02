package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/rbac"
	"github.com/content-services/tang/pkg/tangy"
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
	// Flat cross-repo endpoints
	addRepoRoute(engine, http.MethodGet, "/lightwell/packages", h.listPackages, rbac.RbacVerbRead)
	addRepoRoute(engine, http.MethodGet, "/lightwell/package_versions", h.listPackageVersions, rbac.RbacVerbRead)
	// Nested repo-scoped aliases
	addRepoRoute(engine, http.MethodGet, "/lightwell/repositories/:repository_name/packages", h.listRepoPackages, rbac.RbacVerbRead)
	addRepoRoute(engine, http.MethodGet, "/lightwell/repositories/:repository_name/package_versions", h.listRepoPackageVersions, rbac.RbacVerbRead)
}

// listLightwellPackages godoc
// @Summary      List Lightwell Packages (cross-repo)
// @ID           listLightwellPackages
// @Description  List packages aggregated across all Lightwell repositories, with optional filtering by content type, name, and security level.
// @Tags         lightwell
// @Accept       json
// @Produce      json
// @Param        content_type    query  string  false  "Filter by content type (maven, python, npm)"
// @Param        name            query  string  false  "Filter by package name (substring match)"
// @Param        security_level  query  string  false  "Filter by security level (validated, remediated)"
// @Param        limit           query  int     false  "Limit of results to return"
// @Param        offset          query  int     false  "Offset into results"
// @Success      200 {object} api.LightwellPackageCollectionResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /lightwell/packages [get]
func (h *LightwellPackagesHandler) listPackages(c echo.Context) error {
	page := ParsePagination(c)
	filters := parseLightwellPackageFilters(c)

	if err := validateContentType(filters.ContentType); err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Invalid content_type filter", err.Error())
	}

	repos, err := h.fetchLightwellRepos(c, filters.ContentType, filters.SecurityLevel)
	if err != nil {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Error listing Lightwell repositories", err.Error())
	}
	if filters.Repository != "" {
		repos = filterReposByName(repos, filters.Repository)
	}

	items, err := h.aggregatePackages(c.Request().Context(), repos, filters.Name)
	if err != nil {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Error retrieving packages", err.Error())
	}

	sortLightwellPackages(items, page.SortBy)
	totalCount := int64(len(items))
	paged := paginatePackages(items, page.Offset, page.Limit)
	resp := api.LightwellPackageCollectionResponse{Data: paged}
	collResp := setCollectionResponseMetadata(&resp, c, totalCount)
	return c.JSON(http.StatusOK, collResp)
}

// listLightwellPackageVersions godoc
// @Summary      List Lightwell Package Versions (cross-repo)
// @ID           listLightwellPackageVersions
// @Description  List individual package versions aggregated across all Lightwell repositories, with optional CVE-based filtering.
// @Tags         lightwell
// @Accept       json
// @Produce      json
// @Param        content_type         query  string  false  "Filter by content type (maven, python, npm)"
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
func (h *LightwellPackagesHandler) listPackageVersions(c echo.Context) error {
	page := ParsePagination(c)
	filters := parseLightwellPackageVersionFilters(c)

	if err := validateContentType(filters.ContentType); err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Invalid content_type filter", err.Error())
	}

	repos, err := h.fetchLightwellRepos(c, filters.ContentType, filters.SecurityLevel)
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
	collResp := setCollectionResponseMetadata(&resp, c, totalCount)
	return c.JSON(http.StatusOK, collResp)
}

// fetchLightwellRepos returns Lightwell repos for the caller's org, optionally
// filtered by content type and security level.
func (h *LightwellPackagesHandler) fetchLightwellRepos(c echo.Context, contentType, securityLevel string) ([]api.RepositoryResponse, error) {
	_, orgID := getAccountIdOrgId(c)
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

func (h *LightwellPackagesHandler) fetchPackagesFromRepo(ctx context.Context, repo api.RepositoryResponse, nameSearch string) ([]api.LightwellPackageResponse, error) {
	if repo.PublishedDistBasePath == "" {
		return nil, nil
	}

	repositoryHref, err := h.resolveRepositoryHref(ctx, repo)
	if err != nil {
		return nil, err
	}

	// Fetch all packages from this repo (no server-side pagination — small datasets)
	pageOpts := tangy.PageOptions{Offset: 0, Limit: MaxLimit}

	switch repo.ContentType {
	case config.ContentTypeMaven:
		tangResp, err := h.TangClient.MavenPackageList(ctx, repositoryHref, tangy.MavenPackageListFilters{Search: nameSearch}, pageOpts)
		if err != nil {
			return nil, err
		}
		return mapMavenToLightwellPackages(tangResp, repo), nil

	case config.ContentTypePython:
		tangResp, err := h.TangClient.PythonPackageList(ctx, repositoryHref, tangy.PythonPackageListFilters{Search: nameSearch}, pageOpts)
		if err != nil {
			return nil, err
		}
		return mapPythonToLightwellPackages(tangResp, repo), nil

	case config.ContentTypeNpm:
		tangResp, err := h.TangClient.NpmPackageList(ctx, repositoryHref, tangy.NpmPackageListFilters{Search: nameSearch}, pageOpts)
		if err != nil {
			return nil, err
		}
		return mapNpmToLightwellPackages(tangResp, repo), nil

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

	repositoryHref, err := h.resolveRepositoryHref(ctx, repo)
	if err != nil {
		return nil, err
	}

	pageOpts := tangy.PageOptions{Offset: 0, Limit: MaxLimit}

	switch repo.ContentType {
	case config.ContentTypeMaven:
		tangResp, err := h.TangClient.MavenPackageList(ctx, repositoryHref, tangy.MavenPackageListFilters{Search: nameSearch}, pageOpts)
		if err != nil {
			return nil, err
		}
		return expandMavenVersions(tangResp, repo), nil

	case config.ContentTypePython:
		tangResp, err := h.TangClient.PythonPackageList(ctx, repositoryHref, tangy.PythonPackageListFilters{Search: nameSearch}, pageOpts)
		if err != nil {
			return nil, err
		}
		return expandPythonVersions(tangResp, repo), nil

	case config.ContentTypeNpm:
		tangResp, err := h.TangClient.NpmPackageList(ctx, repositoryHref, tangy.NpmPackageListFilters{Search: nameSearch}, pageOpts)
		if err != nil {
			return nil, err
		}
		return expandNpmVersions(tangResp, repo), nil

	default:
		return nil, nil
	}
}

func (h *LightwellPackagesHandler) resolveRepositoryHref(ctx context.Context, repo api.RepositoryResponse) (string, error) {
	domainName, err := h.DaoRegistry.Domain.FetchOrCreateDomain(ctx, repo.OrgID)
	if err != nil {
		return "", err
	}
	pulpClient := h.PulpClient.WithDomain(domainName)
	href, err := pulpClient.ResolveRepositoryFromBasePath(ctx, repo.PublishedDistBasePath)
	if err != nil {
		return "", fmt.Errorf("repo %s: %w", repo.UUID, err)
	}
	if href == nil {
		return "", fmt.Errorf("repo %s: distribution not found", repo.UUID)
	}
	return *href, nil
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

func mapMavenToLightwellPackages(resp tangy.MavenPackageListResponse, repo api.RepositoryResponse) []api.LightwellPackageResponse {
	out := make([]api.LightwellPackageResponse, 0, len(resp.Results))
	for _, item := range resp.Results {
		releases := make([]api.ReleaseInfo, len(item.LatestReleases))
		for j, rel := range item.LatestReleases {
			releases[j] = api.ReleaseInfo{Version: rel.Version, Release: rel.Release, CreatedAt: rel.CreatedAt}
		}
		out = append(out, api.LightwellPackageResponse{
			Name:           item.ArtifactID,
			Group:          item.GroupID,
			ContentType:    config.ContentTypeMaven,
			Repository:     repo.Name,
			RepositoryUUID: repo.UUID,
			Versions:       item.Versions,
			LatestReleases: releases,
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
			ContentType:    config.ContentTypePython,
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
		scope, name := parseNpmPackageName(item.Name)
		out = append(out, api.LightwellPackageResponse{
			Name:           name,
			Group:          scope,
			ContentType:    config.ContentTypeNpm,
			Repository:     repo.Name,
			RepositoryUUID: repo.UUID,
			Versions:       item.Versions,
			LatestReleases: releases,
		})
	}
	return out
}

func expandMavenVersions(resp tangy.MavenPackageListResponse, repo api.RepositoryResponse) []api.LightwellPackageVersionResponse {
	var out []api.LightwellPackageVersionResponse
	for _, item := range resp.Results {
		relMap := latestReleaseMap(item.LatestReleases)
		for _, v := range item.Versions {
			ver := api.LightwellPackageVersionResponse{
				Name:           item.ArtifactID,
				Group:          item.GroupID,
				Version:        v,
				ContentType:    config.ContentTypeMaven,
				Repository:     repo.Name,
				RepositoryUUID: repo.UUID,
				Purl:           buildPURL(config.ContentTypeMaven, item.GroupID, item.ArtifactID, v),
				Coordinates:    buildCoordinates(config.ContentTypeMaven, item.GroupID, item.ArtifactID),
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
				ContentType:    config.ContentTypePython,
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
		scope, name := parseNpmPackageName(item.Name)
		verMap := npmVersionMap(item.LatestVersions)
		for _, v := range item.Versions {
			ver := api.LightwellPackageVersionResponse{
				Name:           name,
				Group:          scope,
				Version:        v,
				ContentType:    config.ContentTypeNpm,
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
		String("content_type", &f.ContentType).
		String("name", &f.Name).
		String("repository", &f.Repository).
		String("security_level", &f.SecurityLevel).
		BindError()
	return f
}

func parseLightwellPackageVersionFilters(c echo.Context) api.LightwellPackageVersionFilterData {
	var f api.LightwellPackageVersionFilterData
	_ = echo.QueryParamsBinder(c).
		String("content_type", &f.ContentType).
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

func latestReleaseMap(releases []tangy.MavenReleaseInfo) map[string]mavenRelInfo {
	m := make(map[string]mavenRelInfo, len(releases))
	for _, r := range releases {
		m[r.Version] = mavenRelInfo{Release: r.Release, CreatedAt: r.CreatedAt}
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

// --- nested repo-scoped alias handlers ---

func (h *LightwellPackagesHandler) listRepoPackages(c echo.Context) error {
	repoName := c.Param("repository_name")
	c.QueryParams().Set("repository", repoName)
	return h.listPackages(c)
}

func (h *LightwellPackagesHandler) listRepoPackageVersions(c echo.Context) error {
	repoName := c.Param("repository_name")
	c.QueryParams().Set("repository", repoName)
	return h.listPackageVersions(c)
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
		case "name":
			less = items[i].Name < items[j].Name
		case "content_type":
			less = items[i].ContentType < items[j].ContentType
		case "repository":
			less = items[i].Repository < items[j].Repository
		default:
			less = items[i].Name < items[j].Name
		}
		if dir == "desc" {
			return !less
		}
		return less
	})
}

func sortLightwellVersions(items []api.LightwellPackageVersionResponse, sortBy string) {
	field, dir := parseSortBy(sortBy)
	if field == "" {
		field = "name"
	}
	sort.SliceStable(items, func(i, j int) bool {
		var less bool
		switch field {
		case "name":
			less = items[i].Name < items[j].Name
		case "version":
			less = items[i].Version < items[j].Version
		case "content_type":
			less = items[i].ContentType < items[j].ContentType
		case "repository":
			less = items[i].Repository < items[j].Repository
		default:
			less = items[i].Name < items[j].Name
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
