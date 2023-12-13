package middleware

import (
	"mime"
	"net/http"

	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/labstack/echo/v4"
)

const JSONMimeType = "application/json"

func enforceJSONContentTypeSkipper(c echo.Context) bool {
	return c.Request().Body == http.NoBody
}

func EnforceJSONContentType(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if enforceJSONContentTypeSkipper(c) {
			return next(c)
		}
		mediatype, _, err := mime.ParseMediaType(c.Request().Header.Get("Content-Type"))
		if err != nil {
			return ce.NewErrorResponse(http.StatusUnsupportedMediaType, "Error parsing content type", err.Error())
		}
		if mediatype != JSONMimeType {
			return ce.NewErrorResponse(http.StatusUnsupportedMediaType, "Incorrect content type", "Content-Type must be application/json")
		}
		return next(c)
	}
}
