package middleware

import (
	"net/http"

	"github.com/content-services/content-sources-backend/pkg/clients/feature_service_client"
	"github.com/content-services/content-sources-backend/pkg/clients/terms_service_client"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/terms"
	"github.com/labstack/echo/v4"
)

// RequireTermsAcceptance guards Lightwell resources using the same entitlement
// filtering and fail-open policy as the terms check endpoint.
func RequireTermsAcceptance(tsClient terms_service_client.TermsServiceClient, fsClient feature_service_client.FeatureServiceClient) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if len(terms.RequiredEvents(c.Request().Context(), tsClient, fsClient)) > 0 {
				return ce.NewErrorResponse(
					http.StatusPreconditionRequired,
					"Terms acceptance required",
					"You must accept the Lightwell terms of use before accessing this resource. Visit https://console.redhat.com/lightwell to accept.",
				)
			}
			return next(c)
		}
	}
}
