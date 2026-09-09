package handler

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/lightwell/osv"
	"github.com/labstack/echo/v4"
)

// OsvDevHandler serves an osv.dev-compatible mock of our Lightwell advisories under
// /demo/osvdev. It is public (no identity required) and gated by the
// LightwellOsvDemo feature flag. See pkg/lightwell/osv for the data mapping.
type OsvDevHandler struct {
	DaoRegistry dao.DaoRegistry
}

// RegisterOsvDevRoutes registers the osv.dev mock on the raw Echo engine (not the
// versioned API group), so paths are literally rooted at /demo/osvdev with no
// identity/RBAC. SkipMiddleware skips auth for the /demo/osvdev prefix.
func RegisterOsvDevRoutes(engine *echo.Echo, daoReg *dao.DaoRegistry) {
	if engine == nil {
		panic("engine is nil")
	}
	if daoReg == nil {
		panic("daoReg is nil")
	}

	h := OsvDevHandler{DaoRegistry: *daoReg}
	engine.POST("/demo/osvdev/v1/query", h.query, requireOsvDemo)
	engine.POST("/demo/osvdev/v1/querybatch", h.queryBatch, requireOsvDemo)
	engine.GET("/demo/osvdev/v1/vulns/:id", h.getVuln, requireOsvDemo)
	engine.GET("/demo/osvdev/ecosystems.txt", h.ecosystems, requireOsvDemo)
	engine.GET("/demo/osvdev/all.zip", h.exportAll, requireOsvDemo)
	engine.GET("/demo/osvdev/:ecosystem/all.zip", h.exportEcosystem, requireOsvDemo)
	engine.GET("/demo/osvdev/:ecosystem/:id", h.getRecordFile, requireOsvDemo)
}

// requireOsvDemo returns 404 when the demo feature is disabled, so the routes are
// invisible in production. The plain Enabled flag is used (not FeatureAccessible),
// since these routes are public and carry no identity/org.
func requireOsvDemo(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if !config.Get().Features.LightwellOsvDemo.Enabled {
			return ce.NewErrorResponse(http.StatusNotFound, "Not found", "osv.dev demo is not enabled")
		}
		return next(c)
	}
}

func (h *OsvDevHandler) records(c echo.Context) ([]api.OsvVulnerability, error) {
	advisories, err := h.DaoRegistry.LightwellAdvisory.ListForOsv(c.Request().Context())
	if err != nil {
		return nil, ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error loading advisories", err.Error())
	}
	recs := osv.BuildRecords(advisories)
	osv.SortRecords(recs)
	return recs, nil
}

// query implements POST /demo/osvdev/v1/query.
func (h *OsvDevHandler) query(c echo.Context) error {
	var q api.OsvQuery
	if err := c.Bind(&q); err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Invalid query", err.Error())
	}

	recs, err := h.records(c)
	if err != nil {
		return err
	}

	var vulns []api.OsvVulnerability
	for _, rec := range recs {
		if osv.Matches(rec, q) {
			vulns = append(vulns, rec)
		}
	}
	return c.JSON(http.StatusOK, api.OsvVulnerabilityList{Vulns: vulns})
}

// queryBatch implements POST /demo/osvdev/v1/querybatch. Results are index-aligned
// with the request queries and contain only {id, modified} stubs.
func (h *OsvDevHandler) queryBatch(c echo.Context) error {
	var batch api.OsvBatchQuery
	if err := c.Bind(&batch); err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Invalid query", err.Error())
	}

	recs, err := h.records(c)
	if err != nil {
		return err
	}

	results := make([]api.OsvBatchResult, len(batch.Queries))
	for i, q := range batch.Queries {
		var stubs []api.OsvVulnStub
		for _, rec := range recs {
			if osv.Matches(rec, q) {
				stubs = append(stubs, api.OsvVulnStub{ID: rec.ID, Modified: rec.Modified})
			}
		}
		results[i] = api.OsvBatchResult{Vulns: stubs}
	}
	return c.JSON(http.StatusOK, api.OsvBatchVulnerabilityList{Results: results})
}

// getVuln implements GET /demo/osvdev/v1/vulns/:id.
func (h *OsvDevHandler) getVuln(c echo.Context) error {
	id := c.Param("id")
	recs, err := h.records(c)
	if err != nil {
		return err
	}
	for _, rec := range recs {
		if rec.ID == id {
			return c.JSON(http.StatusOK, rec)
		}
	}
	return ce.NewErrorResponse(http.StatusNotFound, "Not found", "no vulnerability with id "+id)
}

// getRecordFile implements GET /demo/osvdev/:ecosystem/:id, serving <id>.json to
// emulate the GCS export layout.
func (h *OsvDevHandler) getRecordFile(c echo.Context) error {
	id := strings.TrimSuffix(c.Param("id"), ".json")
	recs, err := h.records(c)
	if err != nil {
		return err
	}
	for _, rec := range recs {
		if rec.ID == id {
			return c.JSON(http.StatusOK, rec)
		}
	}
	return ce.NewErrorResponse(http.StatusNotFound, "Not found", "no vulnerability with id "+id)
}

// ecosystems implements GET /demo/osvdev/ecosystems.txt.
func (h *OsvDevHandler) ecosystems(c echo.Context) error {
	return c.String(http.StatusOK, osv.DefaultEcosystem+"\n")
}

// exportAll implements GET /demo/osvdev/all.zip.
func (h *OsvDevHandler) exportAll(c echo.Context) error {
	recs, err := h.records(c)
	if err != nil {
		return err
	}
	return h.writeZip(c, recs)
}

// exportEcosystem implements GET /demo/osvdev/:ecosystem/all.zip.
func (h *OsvDevHandler) exportEcosystem(c echo.Context) error {
	ecosystem := c.Param("ecosystem")
	recs, err := h.records(c)
	if err != nil {
		return err
	}
	filtered := make([]api.OsvVulnerability, 0, len(recs))
	for _, rec := range recs {
		if recordInEcosystem(rec, ecosystem) {
			filtered = append(filtered, rec)
		}
	}
	return h.writeZip(c, filtered)
}

func recordInEcosystem(rec api.OsvVulnerability, ecosystem string) bool {
	for _, aff := range rec.Affected {
		if strings.EqualFold(aff.Package.Ecosystem, ecosystem) {
			return true
		}
	}
	return false
}

func (h *OsvDevHandler) writeZip(c echo.Context, recs []api.OsvVulnerability) error {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, rec := range recs {
		w, err := zw.Create(rec.ID + ".json")
		if err != nil {
			return ce.NewErrorResponse(http.StatusInternalServerError, "Error building zip", err.Error())
		}
		body, err := json.MarshalIndent(rec, "", "  ")
		if err != nil {
			return ce.NewErrorResponse(http.StatusInternalServerError, "Error building zip", err.Error())
		}
		if _, err := w.Write(body); err != nil {
			return ce.NewErrorResponse(http.StatusInternalServerError, "Error building zip", err.Error())
		}
	}
	if err := zw.Close(); err != nil {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Error building zip", err.Error())
	}
	return c.Blob(http.StatusOK, "application/zip", buf.Bytes())
}
