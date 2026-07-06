package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/rbac"
	"github.com/content-services/tang/pkg/tangy"
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
	addRepoRoute(engine, http.MethodGet, "/repositories/:uuid/packages", ph.listPackages, rbac.RbacVerbRead)
	addRepoRoute(engine, http.MethodGet, "/repositories/:uuid/maven_packages/:group/:name", ph.listMavenPackageVersions, rbac.RbacVerbRead)
	addRepoRoute(engine, http.MethodGet, "/repositories/:uuid/maven_packages/:group/:name/:version", ph.getMavenPackageDetail, rbac.RbacVerbRead)
	addRepoRoute(engine, http.MethodGet, "/repositories/:uuid/python_packages/:name/:version", ph.getPythonPackageDetail, rbac.RbacVerbRead)
	addRepoRoute(engine, http.MethodGet, "/repositories/:uuid/python_packages/:name", ph.getPythonPackageVersions, rbac.RbacVerbRead)
}

// ListPackages godoc
// @Summary      List Packages
// @ID           listPackages
// @Description  List packages for Maven (group and name) or Python (name) repositories. Returns empty results for other content types.
// @Tags         packages
// @Param        uuid path string true "Repository UUID"
// @Param        offset query int false "Starting point for pagination. Default: 0"
// @Param        limit query int false "Number of items per page. Default: 100"
// @Param        search query string false "Term to filter and retrieve items that match the specified search criteria. For Maven, search term can include name or group. For Python, search term can include name."
// @Accept       json
// @Produce      json
// @Success      200 {object} api.PackageResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /repositories/{uuid}/packages [get]
func (ph *PackageHandler) listPackages(c echo.Context) error {
	listPackagesRequest := api.ListPackagesRequest{}
	if err := c.Bind(&listPackagesRequest); err != nil {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Error binding parameters", err.Error())
	}

	uuid := c.Param("uuid")
	// _, orgID := getAccountIdOrgId(c)
	pageData := ParsePagination(c)
	filterData := listPackagesRequest.Search
	ctx := c.Request().Context()

	repo, err := ph.DaoRegistry.RepositoryConfig.Fetch(ctx, config.LightwellOrg, uuid) // TODO, don't hardcode lightwell org
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error fetching repository", err.Error())
	}

	switch repo.ContentType {
	case config.ContentTypeMaven:
		return ph.listMavenPackages(c, ctx, repo, filterData, pageData)
	case config.ContentTypePython:
		return ph.listPythonPackages(c, ctx, repo, filterData, pageData)
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

	repositoryHref, err := ph.resolveRepositoryHref(ctx, config.LightwellOrg, repo.PublishedDistBasePath, repo.UUID)
	if err != nil {
		return ph.repositoryHrefErrorResponse(err)
	}

	tangResp, err := ph.TangClient.MavenPackageList(
		c.Request().Context(),
		repositoryHref,
		tangy.MavenPackageListFilters{Search: filterData},
		tangy.PageOptions{
			Offset: pageData.Offset,
			Limit:  pageData.Limit,
		},
	)
	if err != nil {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Error retrieving packages", err.Error())
	}

	return c.JSON(http.StatusOK, mapMavenPackagesToAPI(tangResp))
}

func (ph *PackageHandler) listPythonPackages(c echo.Context, ctx context.Context, repo api.RepositoryResponse, filterData string, pageData api.PaginationData) error {
	if repo.PublishedDistBasePath == "" {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Internal Server Error", "Repository distribution base path not available")
	}

	repositoryHref, err := ph.resolveRepositoryHref(ctx, config.LightwellOrg, repo.PublishedDistBasePath, repo.UUID)
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

func (ph *PackageHandler) resolveRepositoryHref(ctx context.Context, orgID, basePath, repoUUID string) (string, error) {
	domainName, err := ph.DaoRegistry.Domain.FetchOrCreateDomain(ctx, orgID)
	if err != nil {
		return "", err
	}

	pulpClient := ph.PulpClient.WithDomain(domainName)
	href, err := pulpClient.ResolveRepositoryFromBasePath(ctx, basePath)
	if err != nil {
		return "", fmt.Errorf("repository for UUID %v: %w", repoUUID, err)
	}
	if href == nil {
		return "", fmt.Errorf("repository for UUID %v: %w", repoUUID, errRepositoryNotFound)
	}

	return *href, nil
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

func mapMavenPackagesToAPI(tangResp tangy.MavenPackageListResponse) api.PackageResponse {
	results := make([]api.PackageItem, len(tangResp.Results))
	for i, item := range tangResp.Results {
		releases := make([]api.ReleaseInfo, len(item.LatestReleases))
		for j, rel := range item.LatestReleases {
			releases[j] = api.ReleaseInfo{
				Version:   rel.Version,
				Release:   rel.Release,
				CreatedAt: rel.CreatedAt,
			}
		}

		results[i] = api.PackageItem{
			Group:          item.GroupID,
			Name:           item.ArtifactID,
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
func (ph *PackageHandler) listMavenPackageVersions(c echo.Context) error {
	uuid := c.Param("uuid")
	groupID := c.Param("group")
	name := c.Param("name")
	ctx := c.Request().Context()

	repo, err := ph.DaoRegistry.RepositoryConfig.Fetch(ctx, config.LightwellOrg, uuid)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error fetching repository", err.Error())
	}

	if repo.ContentType != config.ContentTypeMaven {
		return ce.NewErrorResponse(http.StatusBadRequest, "Bad Request", "Repository is not a Maven repository")
	}

	if repo.PublishedDistBasePath == "" {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Internal Server Error", "Repository distribution base path not available")
	}

	repositoryHref, err := ph.resolveRepositoryHref(ctx, config.LightwellOrg, repo.PublishedDistBasePath, repo.UUID)
	if err != nil {
		return ph.repositoryHrefErrorResponse(err)
	}

	tangResp, err := ph.TangClient.MavenBuildList(ctx, repositoryHref, groupID, name, "", tangy.PageOptions{})
	if err != nil {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Error retrieving package versions", err.Error())
	}

	versions := make([]api.MavenPackageDetailResponse, len(tangResp.Results))
	for i, item := range tangResp.Results {
		versions[i] = api.MavenPackageDetailResponse{
			Group:   groupID,
			Name:    name,
			Version: item.Version,
			Builds: []api.ReleaseInfo{
				{
					Version:   item.Version,
					Release:   item.Release,
					CreatedAt: item.CreatedAt,
				},
			},
		}
	}

	if len(tangResp.Results) > 0 {
		summary, license, projectURL, author, err := ph.mavenPackageMetadata(ctx, groupID, name, tangResp.Results[0].Version)
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
func (ph *PackageHandler) getMavenPackageDetail(c echo.Context) error {
	uuid := c.Param("uuid")
	groupID := c.Param("group")
	name := c.Param("name")
	version := c.Param("version")
	ctx := c.Request().Context()

	repo, err := ph.DaoRegistry.RepositoryConfig.Fetch(ctx, config.LightwellOrg, uuid)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error fetching repository", err.Error())
	}

	if repo.ContentType != config.ContentTypeMaven {
		return ce.NewErrorResponse(http.StatusBadRequest, "Bad Request", "Repository is not a Maven repository")
	}

	if repo.PublishedDistBasePath == "" {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Internal Server Error", "Repository distribution base path not available")
	}

	repositoryHref, err := ph.resolveRepositoryHref(ctx, config.LightwellOrg, repo.PublishedDistBasePath, repo.UUID)
	if err != nil {
		return ph.repositoryHrefErrorResponse(err)
	}

	pageData := ParsePagination(c)
	tangResp, err := ph.TangClient.MavenBuildList(ctx, repositoryHref, groupID, name, version, tangy.PageOptions{
		Offset: pageData.Offset,
		Limit:  pageData.Limit,
	})
	if err != nil {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Error retrieving package builds", err.Error())
	}

	builds := make([]api.ReleaseInfo, len(tangResp.Results))
	for i, item := range tangResp.Results {
		builds[i] = api.ReleaseInfo{
			Version:   item.Version,
			Release:   item.Release,
			CreatedAt: item.CreatedAt,
		}
	}

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
	existing, err := ph.DaoRegistry.MavenPackages.Fetch(ctx, name)
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
func (ph *PackageHandler) getPythonPackageVersions(c echo.Context) error {
	uuid := c.Param("uuid")
	name := c.Param("name")
	ctx := c.Request().Context()

	repo, err := ph.DaoRegistry.RepositoryConfig.Fetch(ctx, config.LightwellOrg, uuid)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error fetching repository", err.Error())
	}

	if repo.ContentType != config.ContentTypePython {
		return ce.NewErrorResponse(http.StatusBadRequest, "Bad Request", "Repository is not a Python repository")
	}

	if repo.PublishedDistBasePath == "" {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Internal Server Error", "Repository distribution base path not available")
	}

	repositoryHref, err := ph.resolveRepositoryHref(ctx, config.LightwellOrg, repo.PublishedDistBasePath, repo.UUID)
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
func (ph *PackageHandler) getPythonPackageDetail(c echo.Context) error {
	uuid := c.Param("uuid")
	name := c.Param("name")
	version := c.Param("version")
	ctx := c.Request().Context()

	repo, err := ph.DaoRegistry.RepositoryConfig.Fetch(ctx, config.LightwellOrg, uuid)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error fetching repository", err.Error())
	}

	if repo.ContentType != config.ContentTypePython {
		return ce.NewErrorResponse(http.StatusBadRequest, "Bad Request", "Repository is not a Python repository")
	}

	if repo.PublishedDistBasePath == "" {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Internal Server Error", "Repository distribution base path not available")
	}

	repositoryHref, err := ph.resolveRepositoryHref(ctx, config.LightwellOrg, repo.PublishedDistBasePath, repo.UUID)
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
