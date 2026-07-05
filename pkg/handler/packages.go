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
	"github.com/content-services/content-sources-backend/pkg/rbac"
	"github.com/content-services/tang/pkg/tangy"
	"github.com/labstack/echo/v4"
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
	addRepoRoute(engine, http.MethodGet, "/repositories/:uuid/maven_packages/:group/:name/:version", ph.getMavenPackageDetail, rbac.RbacVerbRead)
	addRepoRoute(engine, http.MethodGet, "/repositories/:uuid/python_packages/:name/:version", ph.getPythonPackageDetail, rbac.RbacVerbRead)
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
	dist, err := pulpClient.FindGenericDistributionByBasePath(ctx, basePath)
	if err != nil {
		return "", err
	}
	if dist == nil {
		return "", errDistributionNotFound
	}

	repositoryHref := dist.GetRepository()

	// Warning HACK, we are looking up the distribution by base path, and then trying to find the repository from it above,
	//   but some lightwell maven repos use a publication associated with the distribution (no repo link).  However there is no
	//   publication api to pull the publication from. So we must rely on the name of the distribution being the same as the repository,
	//   which for lightwell it will be. Pulp is changing this to not use publications, so this will be temporary, remove after 7/10/2026
	if repositoryHref == "" {
		name := dist.GetName()
		repo, err := pulpClient.FindGenericRepositoryByName(ctx, name)
		if err != nil {
			return "", err
		}
		if repo == nil || repo.PulpHref == nil {
			return "", fmt.Errorf("%w for UUID %v", errRepositoryNotFound, repoUUID)
		}
		repositoryHref = *repo.PulpHref
	}

	return repositoryHref, nil
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

	return c.JSON(http.StatusOK, api.MavenPackageDetailResponse{
		Group:   groupID,
		Name:    name,
		Version: version,
		Builds:  builds,
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
