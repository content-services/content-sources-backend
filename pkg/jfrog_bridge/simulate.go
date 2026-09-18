package jfrog_bridge

import (
	"io"
	"net/http"

	"github.com/content-services/content-sources-backend/pkg/config"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog/log"
)

// simulate runs the full remediation pipeline for a posted CloudEvents body,
// without consuming from Kafka. The body must be the same CloudEvents envelope
// the bridge receives on platform.lightwell.advisory-created (see
// ParseRemediations). Unlike the Kafka path it does not apply the eventtype
// gate, so any ecosystem can be exercised.
//
// Example:
//
//	curl -X POST https://<host>/api/content-sources/v1/admin/jfrog_bridge/simulate/ \
//	  -H 'Content-Type: application/json' \
//	  -H "x-rh-identity: $(echo -n '{"identity":{"type":"Associate","account_number":"11111"}}' | base64 -w0)" \
//	  -d '{
//	    "specversion": "1.0",
//	    "type": "com.redhat.console.lightwelll.lightwell-advisory-created",
//	    "source": "urn:redhat:source:console:app:lightwell",
//	    "eventtype": "java-remediated",
//	    "data": [{
//	      "metadata": {},
//	      "payload": {
//	        "package_name": "org.springframework:spring-core",
//	        "releases": [{
//	          "release_names": [{"name": "5.3.18.rhlw-00003"}],
//	          "related_cve": [{"cve": "CVE-2025-41249", "severity": "important"}]
//	        }]
//	      }
//	    }]
//	  }'
func (h *adminHandler) simulate(c echo.Context) error {
	defer c.Request().Body.Close()
	body, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Error reading body", err.Error())
	}

	remediations, err := ParseRemediations(body)
	if err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Error parsing remediations", err.Error())
	}

	var results []map[string]string
	for _, rem := range remediations {
		gav := gavKey(rem)
		if err := ValidateNotificationCVEs(rem); err != nil {
			log.Warn().Err(err).Str("gav", gav).Msg("embargo check rejected")
			results = append(results, map[string]string{
				"gav":    gav,
				"status": "rejected",
				"error":  err.Error(),
			})
			continue
		}
		if err := h.bridgeHandler.processRemediation(c.Request().Context(), rem); err != nil {
			log.Error().Err(err).Str("gav", gav).Msg("simulate pipeline failed")
			results = append(results, map[string]string{
				"gav":    gav,
				"status": "failed",
				"error":  err.Error(),
			})
			continue
		}
		results = append(results, map[string]string{
			"gav":    gav,
			"status": "success",
		})
	}

	return c.JSON(http.StatusOK, results)
}

func (h *adminHandler) status(c echo.Context) error {
	cfg := config.Get().JFrogBridge
	status := map[string]interface{}{
		"enabled":          cfg.Enabled,
		"catalog_url":      cfg.CatalogURL,
		"catalog_repo":     cfg.CatalogRepo,
		"registry_url":     cfg.RegistryURL,
		"registry_osv_url": cfg.RegistryOSVURL,
	}
	return c.JSON(http.StatusOK, status)
}
