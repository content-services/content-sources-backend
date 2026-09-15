package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/rbac"
	"github.com/content-services/tang/pkg/tangy"
	zest "github.com/content-services/zest/release/v2026"
	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog/log"
)

var errDistributionNotFound = errors.New("repository distribution not found")

var errRepositoryNotFound = errors.New("repository not found")

type PackageHandler struct {
	DaoRegistry dao.DaoRegistry
	TangClient  tangy.Tangy
	PulpClient  pulp_client.PulpClient
}

func RegisterPackageRoutes(engine *echo.Group, daoReg *dao.DaoRegistry, tangClient tangy.Tangy, pulpClient pulp_client.PulpClient) {
	ph := PackageHandler{
		DaoRegistry: *daoReg,
		TangClient:  tangClient,
		PulpClient:  pulpClient,
	}
	addRepoRoute(engine, http.MethodGet, "/repositories/:uuid/packages", ph.ListPackages, rbac.RbacVerbRead)
	addRepoRoute(engine, http.MethodGet, "/repositories/:uuid/maven_packages/:group/:name", ph.ListMavenPackageVersions, rbac.RbacVerbRead)
	addRepoRoute(engine, http.MethodGet, "/repositories/:uuid/maven_packages/:group/:name/:version", ph.GetMavenPackageDetail, rbac.RbacVerbRead)
	addRepoRoute(engine, http.MethodGet, "/repositories/:uuid/python_packages/:name/:version", ph.GetPythonPackageDetail, rbac.RbacVerbRead)
	addRepoRoute(engine, http.MethodGet, "/repositories/:uuid/python_packages/:name", ph.GetPythonPackageVersions, rbac.RbacVerbRead)
	addRepoRoute(engine, http.MethodGet, "/repositories/:uuid/npm_packages/:scope/:name", ph.GetNpmPackageVersions, rbac.RbacVerbRead)
	addRepoRoute(engine, http.MethodGet, "/repositories/:uuid/npm_packages/:scope/:name/:version", ph.GetNpmPackageDetail, rbac.RbacVerbRead)
}

func (ph *PackageHandler) fetchLightwellRepo(c echo.Context, uuid string) (api.RepositoryResponse, error) {
	_, orgID := GetAccountIdOrgId(c)
	ctx := c.Request().Context()

	repos, _, err := ph.DaoRegistry.RepositoryConfig.List(ctx, orgID, api.PaginationData{Limit: 1}, api.FilterData{UUID: uuid})
	if err != nil {
		return api.RepositoryResponse{}, err
	}

	err = &ce.DaoError{
		NotFound: true,
		Message:  "Repository not found",
	}
	if len(repos.Data) == 0 {
		return api.RepositoryResponse{}, err
	}
	return repos.Data[0], nil
}

// ListPackages godoc
// @Summary      List Packages
// @ID           listPackages
// @Description  List packages for Maven (group and name), Python (name), or npm (scope and name) repositories. Returns empty results for other content types.
// @Tags         packages
// @Param        uuid path string true "Repository UUID"
// @Param        offset query int false "Starting point for pagination. Default: 0"
// @Param        limit query int false "Number of items per page. Default: 100"
// @Param        search query string false "Term to filter and retrieve items that match the specified search criteria. For Maven, search term can include name or group. For Python and npm, search term can include name (including scoped names like @types/)."
// @Accept       json
// @Produce      json
// @Success      200 {object} api.PackageResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /repositories/{uuid}/packages [get]
func (ph *PackageHandler) ListPackages(c echo.Context) error {
	listPackagesRequest := api.ListPackagesRequest{}
	if err := c.Bind(&listPackagesRequest); err != nil {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Error binding parameters", err.Error())
	}

	uuid := c.Param("uuid")
	pageData := ParsePagination(c)
	filterData := listPackagesRequest.Search
	ctx := c.Request().Context()

	repo, err := ph.fetchLightwellRepo(c, uuid)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error fetching repository", err.Error())
	}

	switch repo.ContentType {
	case config.ContentTypeMaven:
		return ph.listMavenPackages(c, ctx, repo, filterData, pageData)
	case config.ContentTypePython:
		return ph.listPythonPackages(c, ctx, repo, filterData, pageData)
	case config.ContentTypeNpm:
		return ph.listNpmPackages(c, ctx, repo, filterData, pageData)
	default:
		return c.JSON(http.StatusOK, api.PackageResponse{
			Results: []api.PackageItem{},
			Total:   0,
			Limit:   pageData.Limit,
			Offset:  pageData.Offset,
		})
	}
}

func (ph *PackageHandler) listMavenPackages(c echo.Context, ctx context.Context, repo api.RepositoryResponse, filterData string, pageData api.PaginationData) error {
	if repo.PublishedDistBasePath == "" {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Internal Server Error", "Repository distribution base path not available")
	}

	pulpClient, repositoryHref, err := ph.resolveRepository(ctx, repo.OrgID, repo.PublishedDistBasePath, repo.UUID)
	if err != nil {
		return ph.repositoryHrefErrorResponse(err)
	}

	pulpResp, err := pulpClient.ListMavenPackages(ctx, repositoryHref, filterData, pageData.Limit, pageData.Offset)
	if err != nil {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Error retrieving packages", err.Error())
	}

	return c.JSON(http.StatusOK, mapMavenPackagesToAPI(pulpResp, pageData.Limit, pageData.Offset))
}

func (ph *PackageHandler) listPythonPackages(c echo.Context, ctx context.Context, repo api.RepositoryResponse, filterData string, pageData api.PaginationData) error {
	if repo.PublishedDistBasePath == "" {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Internal Server Error", "Repository distribution base path not available")
	}

	repositoryHref, err := ph.resolveRepositoryHref(ctx, repo.OrgID, repo.PublishedDistBasePath, repo.UUID)
	if err != nil {
		return ph.repositoryHrefErrorResponse(err)
	}

	tangResp, err := ph.TangClient.PythonPackageList(ctx, repositoryHref, tangy.PythonPackageListFilters{Search: filterData}, tangy.PageOptions{
		Offset: pageData.Offset,
		Limit:  pageData.Limit,
	})
	if err != nil {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Error retrieving packages", err.Error())
	}

	return c.JSON(http.StatusOK, mapPythonPackagesToAPI(tangResp))
}

func (ph *PackageHandler) resolveRepository(ctx context.Context, orgID, basePath, repoUUID string) (pulp_client.PulpClient, string, error) {
	domainName, err := ph.DaoRegistry.Domain.FetchOrCreateDomain(ctx, orgID)
	if err != nil {
		return nil, "", err
	}

	pulpClient := ph.PulpClient.WithDomain(domainName)
	href, err := pulpClient.ResolveRepositoryFromBasePath(ctx, basePath)
	if err != nil {
		return nil, "", fmt.Errorf("repository for UUID %v: %w", repoUUID, err)
	}
	if href == nil {
		return nil, "", fmt.Errorf("repository for UUID %v: %w", repoUUID, errRepositoryNotFound)
	}

	return pulpClient, *href, nil
}

func (ph *PackageHandler) resolveRepositoryHref(ctx context.Context, orgID, basePath, repoUUID string) (string, error) {
	_, href, err := ph.resolveRepository(ctx, orgID, basePath, repoUUID)
	return href, err
}

func (ph *PackageHandler) repositoryHrefErrorResponse(err error) error {
	if errors.Is(err, errDistributionNotFound) {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Internal Server Error", "Repository distribution not found")
	}
	if errors.Is(err, errRepositoryNotFound) {
		return ce.NewErrorResponse(http.StatusNotFound, "Repository not found", err.Error())
	}
	var daoError *ce.DaoError
	if errors.As(err, &daoError) {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error fetching or creating domain", err.Error())
	}
	return ce.NewErrorResponse(http.StatusInternalServerError, "Error finding repository distribution", err.Error())
}

func mapMavenPackagesToAPI(pulpResp zest.PaginatedMavenRepositoryPackageListResponse, limit, offset int) api.PackageResponse {
	results := make([]api.PackageItem, len(pulpResp.Results))
	for i, item := range pulpResp.Results {
		results[i] = api.PackageItem{
			Group:          item.GroupId,
			Name:           item.ArtifactId,
			Versions:       item.Versions,
			LatestReleases: mapMavenPackageReleases(item.LatestReleases),
		}
	}

	return api.PackageResponse{
		Results: results,
		Total:   int(pulpResp.Count),
		Limit:   limit,
		Offset:  offset,
	}
}

func mapMavenPackageReleases(releases []zest.MavenPackageReleaseResponse) []api.ReleaseInfo {
	out := make([]api.ReleaseInfo, len(releases))
	for i, rel := range releases {
		out[i] = api.ReleaseInfo{
			Version:   rel.Version,
			Release:   rel.Release,
			CreatedAt: rel.CreatedAt.Format(time.RFC3339),
		}
	}
	return out
}

// Pulp content/maven/package/ returns one MavenPackage unit per rebuild:
//
//	version="5.3.18.rhlw-00003"  base_version="5.3.18"  pulp_created=...
//
// CS versions/detail JSON wants that nested as:
//
//	{ "version": "5.3.18", "builds": [{ "version": "5.3.18", "release": "rhlw-00003", "created_at": "..." }] }
func toMavenBuild(pkg zest.MavenMavenPackageResponse) api.ReleaseInfo {
	base := pkg.GetBaseVersion()
	createdAt := ""
	if pkg.HasPulpCreated() {
		createdAt = pkg.GetPulpCreated().Format(time.RFC3339)
	}
	return api.ReleaseInfo{
		Version:   base,
		Release:   mavenRebuildQualifier(pkg.GetVersion(), base),
		CreatedAt: createdAt,
	}
}

// mavenRebuildQualifier is the rebuild suffix after base_version.
// "5.3.18.rhlw-00003" / "5.3.18" → "rhlw-00003"; equal versions → "".
func mavenRebuildQualifier(fullVersion, baseVersion string) string {
	if baseVersion == "" || fullVersion == baseVersion {
		return ""
	}
	suffix, ok := strings.CutPrefix(fullVersion, baseVersion+".")
	if !ok {
		return ""
	}
	return suffix
}

func newestMavenUnitsFirst(pkgs []zest.MavenMavenPackageResponse) []zest.MavenMavenPackageResponse {
	out := slices.Clone(pkgs)
	slices.SortStableFunc(out, func(a, b zest.MavenMavenPackageResponse) int {
		return b.GetPulpCreated().Compare(a.GetPulpCreated())
	})
	return out
}

func toMavenBuilds(pkgs []zest.MavenMavenPackageResponse) []api.ReleaseInfo {
	pkgs = newestMavenUnitsFirst(pkgs)
	builds := make([]api.ReleaseInfo, len(pkgs))
	for i, pkg := range pkgs {
		builds[i] = toMavenBuild(pkg)
	}
	return builds
}

func toMavenVersions(pkgs []zest.MavenMavenPackageResponse, group, name string) []api.MavenPackageDetailResponse {
	versions := make([]api.MavenPackageDetailResponse, 0)
	seen := make(map[string]int)
	for _, pkg := range newestMavenUnitsFirst(pkgs) {
		build := toMavenBuild(pkg)
		if i, ok := seen[build.Version]; ok {
			versions[i].Builds = append(versions[i].Builds, build)
			continue
		}
		seen[build.Version] = len(versions)
		versions = append(versions, api.MavenPackageDetailResponse{
			Group:   group,
			Name:    name,
			Version: build.Version,
			Builds:  []api.ReleaseInfo{build},
		})
	}
	return versions
}

func mapPythonPackagesToAPI(tangResp tangy.PythonPackageListResponse) api.PackageResponse {
	results := make([]api.PackageItem, len(tangResp.Results))
	for i, item := range tangResp.Results {
		releases := make([]api.ReleaseInfo, len(item.LatestVersions))
		for j, ver := range item.LatestVersions {
			releases[j] = api.ReleaseInfo{
				Version:   ver.Version,
				CreatedAt: ver.CreatedAt,
			}
		}

		results[i] = api.PackageItem{
			Name:           item.NameNormalized,
			Versions:       item.Versions,
			LatestReleases: releases,
		}
	}

	return api.PackageResponse{
		Results: results,
		Total:   tangResp.Total,
		Limit:   tangResp.Limit,
		Offset:  tangResp.Offset,
	}
}

// ListMavenPackageVersions godoc
// @Summary      List Maven Package Versions
// @ID           listMavenPackageVersions
// @Description  List all versions (builds) for a specific Maven package by group and name.
// @Tags         packages
// @Param        uuid path string true "Repository UUID"
// @Param        group path string true "Maven package group ID"
// @Param        name path string true "Maven package artifact ID"
// @Accept       json
// @Produce      json
// @Success      200 {object} api.MavenPackageVersionsResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /repositories/{uuid}/maven_packages/{group}/{name} [get]
func (ph *PackageHandler) ListMavenPackageVersions(c echo.Context) error {
	uuid := c.Param("uuid")
	groupID := c.Param("group")
	name := c.Param("name")
	ctx := c.Request().Context()

	repo, err := ph.fetchLightwellRepo(c, uuid)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error fetching repository", err.Error())
	}

	if repo.ContentType != config.ContentTypeMaven {
		return ce.NewErrorResponse(http.StatusBadRequest, "Bad Request", "Repository is not a Maven repository")
	}

	if repo.PublishedDistBasePath == "" {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Internal Server Error", "Repository distribution base path not available")
	}

	pulpClient, repositoryHref, err := ph.resolveRepository(ctx, repo.OrgID, repo.PublishedDistBasePath, repo.UUID)
	if err != nil {
		return ph.repositoryHrefErrorResponse(err)
	}

	pkgs, err := pulpClient.ListMavenPackageContent(ctx, repositoryHref, groupID, name, "")
	if err != nil {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Error retrieving package versions", err.Error())
	}

	versions := toMavenVersions(pkgs, groupID, name)

	if len(versions) > 0 {
		summary, license, projectURL, author, err := ph.mavenPackageMetadata(ctx, groupID, name, versions[0].Version)
		if err != nil {
			return ce.NewErrorResponse(http.StatusInternalServerError, "Error retrieving package metadata from maven", err.Error())
		}
		for i := range versions {
			versions[i].Summary = summary
			versions[i].License = license
			versions[i].ProjectURL = projectURL
			versions[i].Author = author
		}
	}

	return c.JSON(http.StatusOK, api.MavenPackageVersionsResponse{
		Group:    groupID,
		Name:     name,
		Versions: versions,
	})
}

// GetMavenPackageDetail godoc
// @Summary      Get Maven Package Detail
// @ID           getPackageDetail
// @Description  Get builds for a specific Maven package by group, name, and version.
// @Tags         packages
// @Param        uuid path string true "Repository UUID"
// @Param        group path string true "Maven package group ID"
// @Param        name path string true "Maven package artifact ID"
// @Param        version path string true "Maven package version"
// @Accept       json
// @Produce      json
// @Success      200 {object} api.MavenPackageDetailResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /repositories/{uuid}/maven_packages/{group}/{name}/{version} [get]
func (ph *PackageHandler) GetMavenPackageDetail(c echo.Context) error {
	uuid := c.Param("uuid")
	groupID := c.Param("group")
	name := c.Param("name")
	version := c.Param("version")
	ctx := c.Request().Context()

	repo, err := ph.fetchLightwellRepo(c, uuid)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error fetching repository", err.Error())
	}

	if repo.ContentType != config.ContentTypeMaven {
		return ce.NewErrorResponse(http.StatusBadRequest, "Bad Request", "Repository is not a Maven repository")
	}

	if repo.PublishedDistBasePath == "" {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Internal Server Error", "Repository distribution base path not available")
	}

	pulpClient, repositoryHref, err := ph.resolveRepository(ctx, repo.OrgID, repo.PublishedDistBasePath, repo.UUID)
	if err != nil {
		return ph.repositoryHrefErrorResponse(err)
	}

	pkgs, err := pulpClient.ListMavenPackageContent(ctx, repositoryHref, groupID, name, version)
	if err != nil {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Error retrieving package builds", err.Error())
	}

	builds := toMavenBuilds(pkgs)

	response := api.MavenPackageDetailResponse{
		Group:   groupID,
		Name:    name,
		Version: version,
		Builds:  builds,
	}

	summary, license, projectURL, author, err := ph.mavenPackageMetadata(ctx, groupID, name, version)
	if err != nil {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Error retrieving package metadata from maven", err.Error())
	}
	response.Summary = summary
	response.License = license
	response.ProjectURL = projectURL
	response.Author = author

	return c.JSON(http.StatusOK, response)
}

func (ph *PackageHandler) mavenPackageMetadata(ctx context.Context, groupID, name, version string) (summary, license, projectURL, author *string, err error) {
	existing, err := ph.DaoRegistry.MavenPackages.Fetch(ctx, groupID, name)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	if existing != nil {
		return existing.Summary, existing.License, existing.ProjectURL, existing.Author, nil
	}

	upstreamVersion := stripLightwellVersionSuffix(version)
	if !isValid(groupID) || !isValid(name) || !isValid(upstreamVersion) {
		return nil, nil, nil, nil, nil
	}

	metadata, fetchErr := fetchMavenCentralMetadata(ctx, nil, groupID, name, version)
	if fetchErr == nil || isMavenCentralPomNotFound(fetchErr) {
		if createErr := ph.DaoRegistry.MavenPackages.Create(ctx, &models.MavenPackage{
			GroupID:    groupID,
			Name:       name,
			Summary:    metadata.Summary,
			License:    metadata.License,
			ProjectURL: metadata.ProjectURL,
			Author:     metadata.Author,
		}); createErr != nil {
			log.Warn().Err(createErr).Str("artifact", name).Msg("Failed to cache maven package metadata")
		}
	} else {
		log.Warn().
			Err(fetchErr).
			Str("group", groupID).
			Str("artifact", name).
			Str("version", version).
			Msg("Failed to fetch maven package metadata from Maven Central")
	}

	if fetchErr != nil {
		return nil, nil, nil, nil, nil
	}

	return metadata.Summary, metadata.License, metadata.ProjectURL, metadata.Author, nil
}

// GetPythonPackageVersions godoc
// @Summary      Get Python Package Versions
// @ID           getPythonPackageVersions
// @Description  Get metadata and distributions for all versions of a Python package by name.
// @Tags         packages
// @Param        uuid path string true "Repository UUID"
// @Param        name path string true "Python package normalized name"
// @Accept       json
// @Produce      json
// @Success      200 {object} api.PythonPackageVersionsResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /repositories/{uuid}/python_packages/{name} [get]
func (ph *PackageHandler) GetPythonPackageVersions(c echo.Context) error {
	uuid := c.Param("uuid")
	name := c.Param("name")
	ctx := c.Request().Context()

	repo, err := ph.fetchLightwellRepo(c, uuid)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error fetching repository", err.Error())
	}

	if repo.ContentType != config.ContentTypePython {
		return ce.NewErrorResponse(http.StatusBadRequest, "Bad Request", "Repository is not a Python repository")
	}

	if repo.PublishedDistBasePath == "" {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Internal Server Error", "Repository distribution base path not available")
	}

	repositoryHref, err := ph.resolveRepositoryHref(ctx, repo.OrgID, repo.PublishedDistBasePath, repo.UUID)
	if err != nil {
		return ph.repositoryHrefErrorResponse(err)
	}

	tangResp, err := ph.TangClient.PythonPackageVersionsGet(ctx, repositoryHref, name)
	if err != nil {
		if errors.Is(err, tangy.ErrPythonPackageNotFound) {
			return ce.NewErrorResponse(http.StatusNotFound, "Package not found", err.Error())
		}
		return ce.NewErrorResponse(http.StatusInternalServerError, "Error retrieving package versions", err.Error())
	}

	versions := make([]api.PythonPackageDetailResponse, len(tangResp))
	for i, detail := range tangResp {
		versions[i] = mapPythonPackageDetailToAPI(detail)
	}

	return c.JSON(http.StatusOK, api.PythonPackageVersionsResponse{
		Name:     name,
		Versions: versions,
	})
}

// GetPythonPackageDetail godoc
// @Summary      Get Python Package Detail
// @ID           getPythonPackageDetail
// @Description  Get metadata and distributions for a specific Python package by name and version.
// @Tags         packages
// @Param        uuid path string true "Repository UUID"
// @Param        name path string true "Python package normalized name"
// @Param        version path string true "Python package version"
// @Accept       json
// @Produce      json
// @Success      200 {object} api.PythonPackageDetailResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /repositories/{uuid}/python_packages/{name}/{version} [get]
func (ph *PackageHandler) GetPythonPackageDetail(c echo.Context) error {
	uuid := c.Param("uuid")
	name := c.Param("name")
	version := c.Param("version")
	ctx := c.Request().Context()

	repo, err := ph.fetchLightwellRepo(c, uuid)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error fetching repository", err.Error())
	}

	if repo.ContentType != config.ContentTypePython {
		return ce.NewErrorResponse(http.StatusBadRequest, "Bad Request", "Repository is not a Python repository")
	}

	if repo.PublishedDistBasePath == "" {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Internal Server Error", "Repository distribution base path not available")
	}

	repositoryHref, err := ph.resolveRepositoryHref(ctx, repo.OrgID, repo.PublishedDistBasePath, repo.UUID)
	if err != nil {
		return ph.repositoryHrefErrorResponse(err)
	}

	tangResp, err := ph.TangClient.PythonPackageGet(ctx, repositoryHref, name, version)
	if err != nil {
		if errors.Is(err, tangy.ErrPythonPackageNotFound) {
			return ce.NewErrorResponse(http.StatusNotFound, "Package not found", err.Error())
		}
		return ce.NewErrorResponse(http.StatusInternalServerError, "Error retrieving package detail", err.Error())
	}

	return c.JSON(http.StatusOK, mapPythonPackageDetailToAPI(tangResp))
}

func mapPythonPackageDetailToAPI(tangDetail tangy.PythonPackageDetail) api.PythonPackageDetailResponse {
	distributions := make([]api.PythonDistribution, len(tangDetail.Distributions))
	for i, dist := range tangDetail.Distributions {
		distributions[i] = api.PythonDistribution{
			Name:          dist.Name,
			Filename:      dist.Filename,
			PackageType:   dist.PackageType,
			PythonVersion: dist.PythonVersion,
			Sha256:        dist.Sha256,
			Size:          dist.Size,
			CreatedAt:     dist.CreatedAt,
		}
	}

	return api.PythonPackageDetailResponse{
		Name:        tangDetail.NameNormalized,
		Version:     tangDetail.Version,
		Summary:     tangDetail.Summary,
		Description: tangDetail.Description,
		LastUpdated: tangDetail.LastUpdated,
		License:     tangDetail.License,
		Author: api.PythonPackageAuthor{
			Name:  tangDetail.Author,
			Email: tangDetail.AuthorEmail,
		},
		UpstreamVersions: tangDetail.Versions,
		ProjectURL:       tangDetail.ProjectURL,
		Distributions:    distributions,
	}
}

func npmPackageName(scope, name string) string {
	if scope == "" || scope == "-" {
		return name
	}
	return scope + "/" + name
}

func ParseNpmPackageName(fullName string) (scope, name string) {
	if strings.HasPrefix(fullName, "@") {
		parts := strings.SplitN(fullName, "/", 2)
		if len(parts) == 2 {
			return parts[0], parts[1]
		}
	}
	return "-", fullName
}

func (ph *PackageHandler) listNpmPackages(c echo.Context, ctx context.Context, repo api.RepositoryResponse, filterData string, pageData api.PaginationData) error {
	if repo.PublishedDistBasePath == "" {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Internal Server Error", "Repository distribution base path not available")
	}

	repositoryHref, err := ph.resolveRepositoryHref(ctx, repo.OrgID, repo.PublishedDistBasePath, repo.UUID)
	if err != nil {
		return ph.repositoryHrefErrorResponse(err)
	}

	tangResp, err := ph.TangClient.NpmPackageList(ctx, repositoryHref, tangy.NpmPackageListFilters{Search: filterData}, tangy.PageOptions{
		Offset: pageData.Offset,
		Limit:  pageData.Limit,
	})
	if err != nil {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Error retrieving packages", err.Error())
	}

	return c.JSON(http.StatusOK, mapNpmPackagesToAPI(tangResp))
}

func mapNpmPackagesToAPI(tangResp tangy.NpmPackageListResponse) api.PackageResponse {
	results := make([]api.PackageItem, len(tangResp.Results))
	for i, item := range tangResp.Results {
		scope, name := ParseNpmPackageName(item.Name)
		releases := make([]api.ReleaseInfo, len(item.LatestVersions))
		for j, ver := range item.LatestVersions {
			releases[j] = api.ReleaseInfo{
				Version:   ver.Version,
				CreatedAt: ver.CreatedAt,
			}
		}

		results[i] = api.PackageItem{
			Group:          scope,
			Name:           name,
			Versions:       item.Versions,
			LatestReleases: releases,
		}
	}

	return api.PackageResponse{
		Results: results,
		Total:   tangResp.Total,
		Limit:   tangResp.Limit,
		Offset:  tangResp.Offset,
	}
}

// GetNpmPackageVersions godoc
// @Summary      Get NPM Package Versions
// @ID           getNpmPackageVersions
// @Description  Get tarball info for all versions of an npm package by scope and name. Use "-" as the scope for unscoped packages.
// @Tags         packages
// @Param        uuid path string true "Repository UUID"
// @Param        scope path string true "NPM package scope (e.g. @types). Use - for unscoped packages."
// @Param        name path string true "NPM package name"
// @Accept       json
// @Produce      json
// @Success      200 {object} api.NpmPackageVersionsResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /repositories/{uuid}/npm_packages/{scope}/{name} [get]
func (ph *PackageHandler) GetNpmPackageVersions(c echo.Context) error {
	uuid := c.Param("uuid")
	scope := c.Param("scope")
	name := c.Param("name")
	packageName := npmPackageName(scope, name)
	ctx := c.Request().Context()

	repo, err := ph.fetchLightwellRepo(c, uuid)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error fetching repository", err.Error())
	}

	if repo.ContentType != config.ContentTypeNpm {
		return ce.NewErrorResponse(http.StatusBadRequest, "Bad Request", "Repository is not an npm repository")
	}

	if repo.PublishedDistBasePath == "" {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Internal Server Error", "Repository distribution base path not available")
	}

	repositoryHref, err := ph.resolveRepositoryHref(ctx, repo.OrgID, repo.PublishedDistBasePath, repo.UUID)
	if err != nil {
		return ph.repositoryHrefErrorResponse(err)
	}

	tangResp, err := ph.TangClient.NpmPackageVersionsGet(ctx, repositoryHref, packageName)
	if err != nil {
		if errors.Is(err, tangy.ErrNpmPackageNotFound) {
			return ce.NewErrorResponse(http.StatusNotFound, "Package not found", err.Error())
		}
		return ce.NewErrorResponse(http.StatusInternalServerError, "Error retrieving package versions", err.Error())
	}

	versions := make([]api.NpmPackageDetailResponse, len(tangResp))
	for i, detail := range tangResp {
		versions[i] = mapNpmPackageDetailToAPI(detail, scope, name)
	}

	return c.JSON(http.StatusOK, api.NpmPackageVersionsResponse{
		Scope:    scope,
		Name:     name,
		Versions: versions,
	})
}

// GetNpmPackageDetail godoc
// @Summary      Get NPM Package Detail
// @ID           getNpmPackageDetail
// @Description  Get tarball info for a specific npm package by scope, name, and version. Use "-" as the scope for unscoped packages.
// @Tags         packages
// @Param        uuid path string true "Repository UUID"
// @Param        scope path string true "NPM package scope (e.g. @types). Use - for unscoped packages."
// @Param        name path string true "NPM package name"
// @Param        version path string true "NPM package version"
// @Accept       json
// @Produce      json
// @Success      200 {object} api.NpmPackageDetailResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /repositories/{uuid}/npm_packages/{scope}/{name}/{version} [get]
func (ph *PackageHandler) GetNpmPackageDetail(c echo.Context) error {
	uuid := c.Param("uuid")
	scope := c.Param("scope")
	name := c.Param("name")
	version := c.Param("version")
	packageName := npmPackageName(scope, name)
	ctx := c.Request().Context()

	repo, err := ph.fetchLightwellRepo(c, uuid)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error fetching repository", err.Error())
	}

	if repo.ContentType != config.ContentTypeNpm {
		return ce.NewErrorResponse(http.StatusBadRequest, "Bad Request", "Repository is not an npm repository")
	}

	if repo.PublishedDistBasePath == "" {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Internal Server Error", "Repository distribution base path not available")
	}

	repositoryHref, err := ph.resolveRepositoryHref(ctx, repo.OrgID, repo.PublishedDistBasePath, repo.UUID)
	if err != nil {
		return ph.repositoryHrefErrorResponse(err)
	}

	tangResp, err := ph.TangClient.NpmPackageGet(ctx, repositoryHref, packageName, version)
	if err != nil {
		if errors.Is(err, tangy.ErrNpmPackageNotFound) {
			return ce.NewErrorResponse(http.StatusNotFound, "Package not found", err.Error())
		}
		return ce.NewErrorResponse(http.StatusInternalServerError, "Error retrieving package detail", err.Error())
	}

	return c.JSON(http.StatusOK, mapNpmPackageDetailToAPI(tangResp, scope, name))
}

func mapNpmPackageDetailToAPI(tangDetail tangy.NpmPackageDetail, scope, name string) api.NpmPackageDetailResponse {
	latestVersions := make([]api.ReleaseInfo, len(tangDetail.LatestVersions))
	for i, ver := range tangDetail.LatestVersions {
		latestVersions[i] = api.ReleaseInfo{
			Version:   ver.Version,
			CreatedAt: ver.CreatedAt,
		}
	}

	return api.NpmPackageDetailResponse{
		Scope:     scope,
		Name:      name,
		Version:   tangDetail.Version,
		CreatedAt: tangDetail.CreatedAt,
		Tarball: api.NpmTarball{
			RelativePath: tangDetail.Tarball.RelativePath,
			Filename:     tangDetail.Tarball.Filename,
			Sha256:       tangDetail.Tarball.Sha256,
			Size:         tangDetail.Tarball.Size,
		},
		UpstreamVersions: tangDetail.Versions,
		LatestVersions:   latestVersions,
	}
}
