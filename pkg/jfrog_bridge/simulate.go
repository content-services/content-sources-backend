package jfrog_bridge

import (
	"io"
	"net/http"

	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog/log"
)

// simulate runs the partner's full remediation pipeline for a posted
// CloudEvents body, without consuming from Kafka. The body must be the same CloudEvents envelope
// the bridge receives on platform.lightwell.advisory-created (see
// ParseRemediations). Unlike the Kafka path it does not apply the eventtype
// gate, so any ecosystem can be exercised.
//
// Example:
//
//	curl -X POST https://<host>/api/content-sources/v1/admin/bridge/<partner>/simulate/ \
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

	bh := bridgeFromContext(c)
	var results []map[string]string
	for _, rem := range remediations {
		gav := gavKey(rem)
		if err := ValidateNotificationCVEs(rem); err != nil {
			log.Warn().Err(err).Str("partner", bh.partner.Name()).Str("gav", gav).Msg("embargo check rejected")
			results = append(results, map[string]string{
				"gav":    gav,
				"status": "rejected",
				"error":  err.Error(),
			})
			continue
		}
		if err := bh.process(c.Request().Context(), rem); err != nil {
			log.Error().Err(err).Str("partner", bh.partner.Name()).Str("gav", gav).Msg("simulate pipeline failed")
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
	return c.JSON(http.StatusOK, bridgeFromContext(c).partner.Status())
}
