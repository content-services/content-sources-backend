package handler

import (
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/lightwell/coords"
	zest "github.com/content-services/zest/release/v2026"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog/log"
)

var pythonFilePackageName = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9._-]*[A-Za-z0-9])?$`)

// ListPythonPackageFiles godoc
// @Summary      List Python Package Files
// @ID           listPythonPackageFiles
// @Description  List actual Python distribution files for one exact full release, including its build suffix. Downloads and security artifacts are not included. Unknown metadata is null. Uses the complete Pulp inventory independently of the package mirror.
// @Tags         packages
// @Param        uuid path string true "Repository UUID"
// @Param        name path string true "Python package name (PEP 503 normalized)"
// @Param        version path string true "Exact full Python version; encode + as %2B"
// @Param        limit query int false "Default 100; zero uses default; positive maximum 200; any negative value is unlimited"
// @Param        offset query int false "Default 0; negative values select from the beginning"
// @Param        sort_by query string false "filename, filename:asc, or filename:desc (default filename:asc)"
// @Accept       json
// @Produce      json
// @Success      200 {object} api.PythonPackageFileCollectionResponse
// @Failure      400 {object} ce.ErrorResponse
// @Failure      401 {object} ce.ErrorResponse
// @Failure      403 {object} ce.ErrorResponse
// @Failure      404 {object} ce.ErrorResponse
// @Failure      428 {object} ce.ErrorResponse
// @Failure      500 {object} ce.ErrorResponse
// @Router       /repositories/{uuid}/python_packages/{name}/{version}/files [get]
func (ph *PackageHandler) ListPythonPackageFiles(c echo.Context) error {
	repoUUID, err := pythonFilesPathParam(c, "uuid")
	if err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Bad Request", "Invalid repository UUID")
	}
	if _, err := uuid.Parse(repoUUID); err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Bad Request", "Invalid repository UUID")
	}
	name, nameErr := pythonFilesPathParam(c, "name")
	version, versionErr := pythonFilesPathParam(c, "version")
	if nameErr != nil || versionErr != nil || !pythonFilePackageName.MatchString(name) || version == "" {
		return ce.NewErrorResponse(http.StatusBadRequest, "Bad Request", "A valid Python package name and exact version are required")
	}
	name = coords.NormalizePythonName(name)
	page := ParsePagination(c)
	switch page.SortBy {
	case "", "filename", "filename:asc", "filename:desc":
	default:
		return ce.NewErrorResponse(http.StatusBadRequest, "Invalid sort_by", "Use filename, filename:asc, or filename:desc")
	}

	repo, err := ph.fetchLightwellRepo(c, repoUUID)
	if err != nil {
		var daoErr *ce.DaoError
		if errors.As(err, &daoErr) && daoErr.NotFound {
			return ce.NewErrorResponse(http.StatusNotFound, "Repository not found", "Repository not found")
		}
		return pythonFilesServerError(c, err)
	}
	if repo.Origin != config.OriginLightwell {
		return ce.NewErrorResponse(http.StatusNotFound, "Repository not found", "Repository not found")
	}
	if repo.ContentType != config.ContentTypePython {
		return ce.NewErrorResponse(http.StatusBadRequest, "Bad Request", "Repository is not a Python repository")
	}
	if repo.OrgID == "" || repo.PublishedDistBasePath == "" {
		return pythonFilesServerError(c, errors.New("missing Python repository owner or distribution mapping"))
	}
	ctx := c.Request().Context()
	// The visible repository can belong to a shared or demo org. Read its
	// trusted owner mapping; never create a domain or use the caller's org.
	domain, err := ph.DaoRegistry.Domain.Fetch(ctx, repo.OrgID)
	if err != nil {
		return pythonFilesServerError(c, err)
	}
	if domain == "" {
		return pythonFilesServerError(c, errors.New("missing Python repository domain mapping"))
	}
	files, err := ph.PulpClient.WithDomain(domain).ListPythonPackageFiles(ctx, repo.PublishedDistBasePath, name, version)
	if err != nil {
		return pythonFilesServerError(c, err)
	}
	if len(files) == 0 {
		return ce.NewErrorResponse(http.StatusNotFound, "Package release not found", "No files were found for the requested package release.")
	}
	// The adapter has already validated, deduplicated, and sorted every page.
	if page.SortBy == "filename:desc" {
		slices.Reverse(files)
	}
	count := len(files)
	start := max(0, min(page.Offset, count))
	end := count
	if page.Limit > 0 && page.Limit < count-start {
		end = start + page.Limit
	}
	response := api.PythonPackageFileCollectionResponse{Data: make([]api.PythonPackageFileResponse, 0, end-start)}
	for _, file := range files[start:end] {
		response.Data = append(response.Data, pythonPackageFileResponse(file))
	}
	SetCollectionResponseMetadataForPage(&response, c, page, int64(count))
	return c.JSON(http.StatusOK, response)
}

func pythonFilesPathParam(c echo.Context, key string) (string, error) {
	value := c.Param(key)
	// Echo routes against RawPath when present, otherwise against the already
	// decoded Path. Only decode in the former case to avoid a second unescape.
	if c.Request().URL.RawPath != "" {
		return url.PathUnescape(value)
	}
	return value, nil
}

func pythonFilesServerError(c echo.Context, err error) error {
	// Adapter errors intentionally retain internal URLs and upstream bodies.
	log.Ctx(c.Request().Context()).Error().Err(err).Msg("Error listing Python package files")
	return ce.NewErrorResponse(http.StatusInternalServerError, "Error listing package files", "Unable to retrieve package files.")
}

func pythonPackageFileResponse(file zest.PythonPythonPackageContentResponse) api.PythonPackageFileResponse {
	packagetype := file.GetPackagetype()
	if packagetype == "" {
		switch {
		case strings.HasSuffix(file.GetFilename(), ".whl"):
			packagetype = "bdist_wheel"
		case strings.HasSuffix(file.GetFilename(), ".tar.gz"):
			packagetype = "sdist"
		}
	}
	item := api.PythonPackageFileResponse{
		Filename: file.GetFilename(), Category: "unknown", Size: file.Size, Sha256: file.Sha256,
	}
	if packagetype != "" {
		item.Packagetype = &packagetype
	}
	if packagetype == "bdist_wheel" || packagetype == "sdist" {
		item.Category = "package"
	}
	return item
}
