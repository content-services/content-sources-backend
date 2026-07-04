package handler

import (
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
	addRepoRoute(engine, http.MethodGet, "/repositories/:uuid/maven_packages/:group/:name/:version", ph.getPackageDetail, rbac.RbacVerbRead)
}

// ListPackages godoc
// @Summary      List Packages
// @ID           listPackages
// @Description  Get packages for a Maven repository grouped by group_id and artifact_id. Returns empty results for non-Maven repositories.
// @Tags         packages
// @Param        uuid path string true "Repository UUID"
// @Param        offset query int false "Starting point for pagination. Default: 0"
// @Param        limit query int false "Number of items per page. Default: 100"
// @Accept       json
// @Produce      json
// @Success      200 {object} api.PackageResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /repositories/{uuid}/packages [get]
func (ph *PackageHandler) listPackages(c echo.Context) error {
	uuid := c.Param("uuid")
	// _, orgID := getAccountIdOrgId(c)
	pageData := ParsePagination(c)

	// Fetch repository config to get content type and distribution URL
	repo, err := ph.DaoRegistry.RepositoryConfig.Fetch(c.Request().Context(), config.LightwellOrg, uuid) // TODO, don't hardcode lightwell org
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error fetching repository", err.Error())
	}

	// Return empty results for non-Maven repositories
	if repo.ContentType != config.ContentTypeMaven {
		return c.JSON(http.StatusOK, api.PackageResponse{
			Results: []api.PackageItem{},
			Total:   0,
			Limit:   pageData.Limit,
			Offset:  pageData.Offset,
		})
	}

	// Check if repository has published distribution base path
	if repo.PublishedDistBasePath == "" {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Internal Server Error", "Repository distribution base path not available")
	}

	// Get domain name for the organization
	domainName, err := ph.DaoRegistry.Domain.FetchOrCreateDomain(c.Request().Context(), config.LightwellOrg)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error fetching or creating domain", err.Error())
	}

	// Get repository href from distribution base path
	pulpClient := ph.PulpClient.WithDomain(domainName)
	dist, err := pulpClient.FindGenericDistributionByBasePath(c.Request().Context(), repo.PublishedDistBasePath)
	if err != nil {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Error finding repository distribution", err.Error())
	}
	if dist == nil {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Internal Server Error", "Repository distribution not found")
	}
	repositoryHref := dist.GetRepository()

	// Call tang to get Maven packages
	tangResp, err := ph.TangClient.MavenPackageList(c.Request().Context(), repositoryHref, tangy.PageOptions{
		Offset: pageData.Offset,
		Limit:  pageData.Limit,
	})
	if err != nil {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Error retrieving packages", err.Error())
	}

	// Transform tang response to API response
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

	return c.JSON(http.StatusOK, api.PackageResponse{
		Results: results,
		Total:   tangResp.Total,
		Limit:   tangResp.Limit,
		Offset:  tangResp.Offset,
	})
}

// GetPackageDetail godoc
// @Summary      Get Package Detail
// @ID           getPackageDetail
// @Description  Get builds for a specific Maven package by group, name, and version.
// @Tags         packages
// @Param        uuid path string true "Repository UUID"
// @Param        group path string true "Maven package group ID"
// @Param        name path string true "Maven package artifact ID"
// @Param        version path string true "Maven package version"
// @Accept       json
// @Produce      json
// @Success      200 {object} api.PackageDetailResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /repositories/{uuid}/maven_packages/{group}/{name}/{version} [get]
func (ph *PackageHandler) getPackageDetail(c echo.Context) error {
	uuid := c.Param("uuid")
	groupID := c.Param("group")
	name := c.Param("name")
	version := c.Param("version")

	repo, err := ph.DaoRegistry.RepositoryConfig.Fetch(c.Request().Context(), config.LightwellOrg, uuid)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error fetching repository", err.Error())
	}

	if repo.ContentType != config.ContentTypeMaven {
		return ce.NewErrorResponse(http.StatusBadRequest, "Bad Request", "Repository is not a Maven repository")
	}

	if repo.PublishedDistBasePath == "" {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Internal Server Error", "Repository distribution base path not available")
	}

	domainName, err := ph.DaoRegistry.Domain.FetchOrCreateDomain(c.Request().Context(), config.LightwellOrg)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error fetching or creating domain", err.Error())
	}

	pulpClient := ph.PulpClient.WithDomain(domainName)
	dist, err := pulpClient.FindGenericDistributionByBasePath(c.Request().Context(), repo.PublishedDistBasePath)
	if err != nil {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Error finding repository distribution", err.Error())
	}
	if dist == nil {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Internal Server Error", "Repository distribution not found")
	}
	repositoryHref := dist.GetRepository()

	pageData := ParsePagination(c)
	tangResp, err := ph.TangClient.MavenBuildList(c.Request().Context(), repositoryHref, groupID, name, version, tangy.PageOptions{
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

	return c.JSON(http.StatusOK, api.PackageDetailResponse{
		Group:   groupID,
		Name:    name,
		Version: version,
		Builds:  builds,
	})
}
