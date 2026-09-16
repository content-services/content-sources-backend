package handler

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/lightwell/osv"
	"github.com/labstack/echo/v4"
)

// OsvDevHandler serves an osv.dev-compatible mock of curated Lightwell OSV records
// under /demo/osvdev. It is public (no identity required) and gated by the
// LightwellOsvDemo feature flag. The records are static, embedded JSON served
// verbatim; see pkg/lightwell/osv for the data and matching logic.
type OsvDevHandler struct{}

// RegisterOsvDevRoutes registers the osv.dev mock on the raw Echo engine (not the
// versioned API group), so paths are literally rooted at /demo/osvdev with no
// identity/RBAC. SkipMiddleware skips auth for the /demo/osvdev prefix.
func RegisterOsvDevRoutes(engine *echo.Echo) {
	if engine == nil {
		panic("engine is nil")
	}

	h := OsvDevHandler{}
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

// query implements POST /demo/osvdev/v1/query.
func (h *OsvDevHandler) query(c echo.Context) error {
	var q api.OsvQuery
	if err := c.Bind(&q); err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Invalid query", err.Error())
	}

	var vulns []json.RawMessage
	for _, rec := range osv.Records() {
		if osv.Matches(rec, q) {
			vulns = append(vulns, rec.Raw)
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

	results := make([]api.OsvBatchResult, len(batch.Queries))
	for i, q := range batch.Queries {
		var stubs []api.OsvVulnStub
		for _, rec := range osv.Records() {
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
	if rec, ok := osv.RecordByID(id); ok {
		return c.JSONBlob(http.StatusOK, rec.Raw)
	}
	return ce.NewErrorResponse(http.StatusNotFound, "Not found", "no vulnerability with id "+id)
}

// getRecordFile implements GET /demo/osvdev/:ecosystem/:id, serving <id>.json to
// emulate the GCS export layout.
func (h *OsvDevHandler) getRecordFile(c echo.Context) error {
	id := strings.TrimSuffix(c.Param("id"), ".json")
	ecosystem := c.Param("ecosystem")
	if rec, ok := osv.RecordByID(id); ok && rec.InEcosystem(ecosystem) {
		return c.JSONBlob(http.StatusOK, rec.Raw)
	}
	return ce.NewErrorResponse(http.StatusNotFound, "Not found", "no vulnerability with id "+id)
}

// ecosystems implements GET /demo/osvdev/ecosystems.txt.
func (h *OsvDevHandler) ecosystems(c echo.Context) error {
	return c.String(http.StatusOK, strings.Join(osv.Ecosystems(), "\n")+"\n")
}

// exportAll implements GET /demo/osvdev/all.zip.
func (h *OsvDevHandler) exportAll(c echo.Context) error {
	return h.writeZip(c, osv.Records())
}

// exportEcosystem implements GET /demo/osvdev/:ecosystem/all.zip.
func (h *OsvDevHandler) exportEcosystem(c echo.Context) error {
	ecosystem := c.Param("ecosystem")
	filtered := make([]osv.Record, 0)
	for _, rec := range osv.Records() {
		if rec.InEcosystem(ecosystem) {
			filtered = append(filtered, rec)
		}
	}
	return h.writeZip(c, filtered)
}

func (h *OsvDevHandler) writeZip(c echo.Context, recs []osv.Record) error {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, rec := range recs {
		w, err := zw.Create(rec.ID + ".json")
		if err != nil {
			return ce.NewErrorResponse(http.StatusInternalServerError, "Error building zip", err.Error())
		}
		if _, err := w.Write(rec.Raw); err != nil {
			return ce.NewErrorResponse(http.StatusInternalServerError, "Error building zip", err.Error())
		}
	}
	if err := zw.Close(); err != nil {
		return ce.NewErrorResponse(http.StatusInternalServerError, "Error building zip", err.Error())
	}
	return c.Blob(http.StatusOK, "application/zip", buf.Bytes())
}
