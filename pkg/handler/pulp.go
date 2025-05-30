package handler

import (
	"io"
	"mime/multipart"
	"net/http"
	"os"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/dao"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/rbac"
	zest "github.com/content-services/zest/release/v2025"
	"github.com/labstack/echo/v4"
)

type PulpHandler struct {
	DaoRegistry dao.DaoRegistry
}

func RegisterPulpRoutes(engine *echo.Group, daoReg *dao.DaoRegistry) {
	if engine == nil {
		panic("engine is nil")
	}
	if daoReg == nil {
		panic("daoReg is nil")
	}

	pulpHandler := PulpHandler{
		DaoRegistry: *daoReg,
	}
	addRepoRoute(engine, http.MethodPost, "/pulp/uploads/", pulpHandler.createUpload, rbac.RbacVerbUpload)
	addRepoRoute(engine, http.MethodPut, "/pulp/uploads/:upload_href", pulpHandler.uploadChunk, rbac.RbacVerbUpload)
	addRepoRoute(engine, http.MethodPost, "/pulp/uploads/:upload_href", pulpHandler.finishUpload, rbac.RbacVerbUpload)
	addRepoRoute(engine, http.MethodGet, "/pulp/tasks/:task_href", pulpHandler.getTask, rbac.RbacVerbRead)
}

func (ph *PulpHandler) createUploadInternal(c echo.Context, request api.CreateUploadRequest) (*zest.UploadResponse, error) {
	_, orgId := getAccountIdOrgId(c)

	if request.Size <= 0 {
		return nil, ce.NewErrorResponse(http.StatusBadRequest, "error creating upload", "upload size must be greater than 0")
	}

	domainName, err := ph.DaoRegistry.Domain.FetchOrCreateDomain(c.Request().Context(), orgId)
	if err != nil {
		return nil, ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "error fetching or creating domain", err.Error())
	}
	pulpClient := pulp_client.GetPulpClientWithDomain(domainName)

	apiResponse, code, err := pulpClient.CreateUpload(c.Request().Context(), request.Size)
	if err != nil {
		return nil, ce.NewErrorResponse(code, "error creating upload", err.Error())
	}

	// Get the upload uuid to put in the upload db
	uploadUuid := ""
	if apiResponse != nil && apiResponse.PulpHref != nil {
		uploadUuid = extractUploadUuid(*apiResponse.PulpHref)
	}

	// Associate the file uploaduuid for later use
	err = ph.DaoRegistry.Uploads.StoreFileUpload(c.Request().Context(), orgId, uploadUuid, request.Sha256, request.ChunkSize, request.Size)

	if err != nil {
		return nil, err
	}

	return apiResponse, nil
}

func (ph *PulpHandler) createUpload(c echo.Context) error {
	var req api.CreateUploadRequest

	if err := c.Bind(&req); err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Error binding parameters", err.Error())
	}
	apiResponse, err := ph.createUploadInternal(c, req)
	if err != nil {
		return err
	}

	return c.JSON(200, apiResponse)
}

func (ph *PulpHandler) uploadChunkInternal(c echo.Context) (*zest.UploadResponse, error) {
	_, orgId := getAccountIdOrgId(c)
	dataInput := api.PulpUploadChunkRequest{}
	if err := c.Bind(&dataInput); err != nil {
		return nil, ce.NewErrorResponse(http.StatusBadRequest, "Error binding parameters", err.Error())
	}

	sha256 := c.FormValue("sha256")
	file, err := c.FormFile("file")
	if err != nil {
		return nil, ce.NewErrorResponse(http.StatusBadRequest, "error retrieving file from request", err.Error())
	}

	// convert file part from request to temp file
	tempFile, err := getFile(file)
	if err != nil {
		return nil, ce.NewErrorResponse(http.StatusInternalServerError, "error converting file from request", err.Error())
	}

	// close and remove the temp file on exit
	defer tempFile.Close()
	defer os.Remove(tempFile.Name())

	domainName, err := ph.DaoRegistry.Domain.Fetch(c.Request().Context(), orgId)
	if err != nil {
		return nil, ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "error fetching domain", err.Error())
	}
	pulpClient := pulp_client.GetPulpClientWithDomain(domainName)

	apiResponse, code, err := pulpClient.UploadChunk(c.Request().Context(), dataInput.UploadHref, c.Request().Header.Get("Content-Range"), tempFile, sha256)
	if err != nil {
		return nil, ce.NewErrorResponse(code, "error uploading chunk", err.Error())
	}

	return apiResponse, nil
}

func (ph *PulpHandler) uploadChunk(c echo.Context) error {
	apiResponse, err := ph.uploadChunkInternal(c)
	if err != nil {
		return err
	}

	return c.JSON(200, apiResponse)
}

func (ph *PulpHandler) finishUpload(c echo.Context) error {
	_, orgId := getAccountIdOrgId(c)
	dataInput := api.FinishUploadRequest{}
	if err := c.Bind(&dataInput); err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Error binding parameters", err.Error())
	}

	domainName, err := ph.DaoRegistry.Domain.Fetch(c.Request().Context(), orgId)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "error fetching domain", err.Error())
	}
	pulpClient := pulp_client.GetPulpClientWithDomain(domainName)

	apiResponse, code, err := pulpClient.FinishUpload(c.Request().Context(), dataInput.UploadHref, dataInput.Sha256)
	if err != nil {
		return ce.NewErrorResponse(code, "error finishing upload", err.Error())
	}

	return c.JSON(200, apiResponse)
}

func (ph *PulpHandler) getTask(c echo.Context) error {
	_, orgId := getAccountIdOrgId(c)
	dataInput := api.TaskRequest{}
	if err := c.Bind(&dataInput); err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Error binding parameters", err.Error())
	}

	domainName, err := ph.DaoRegistry.Domain.Fetch(c.Request().Context(), orgId)
	if err != nil {
		return ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "error fetching domain", err.Error())
	}
	pulpClient := pulp_client.GetPulpClientWithDomain(domainName)

	apiResponse, err := pulpClient.GetTask(c.Request().Context(), dataInput.TaskHref)
	if err != nil {
		return ce.NewErrorResponse(http.StatusInternalServerError, "error fetching pulp task", err.Error())
	}

	return c.JSON(200, apiResponse)
}

func getFile(fileHeader *multipart.FileHeader) (*os.File, error) {
	srcFile, err := fileHeader.Open()
	if err != nil {
		return nil, ce.NewErrorResponse(http.StatusInternalServerError, "error opening file from request", err.Error())
	}
	defer srcFile.Close()

	// copy the contents over to a temp file to create an os.File
	tempFile, err := os.CreateTemp("", "upload-*")
	if err != nil {
		return nil, ce.NewErrorResponse(http.StatusInternalServerError, "error creating temporary file", err.Error())
	}
	_, err = io.Copy(tempFile, srcFile)
	if err != nil {
		return nil, ce.NewErrorResponse(http.StatusInternalServerError, "error copying file content to temporary file", err.Error())
	}

	// reset file pointer to beginning of file
	_, err = tempFile.Seek(0, 0)
	if err != nil {
		return nil, ce.NewErrorResponse(http.StatusInternalServerError, "error resetting file pointer", err.Error())
	}

	return tempFile, nil
}
