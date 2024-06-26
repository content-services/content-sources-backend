package middleware

import (
	"mime"
	"net/http"

	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/labstack/echo/v4"
)

const (
	JSONMimeType       = "application/json"
	FormDataMimeType   = "multipart/form-data"
	URLEncodedMimeType = "application/x-www-form-urlencoded"
)

func enforceJSONContentTypeSkipper(c echo.Context) bool {
	contentType := c.Request().Header.Get("Content-Type")
	if contentType != "" {
		mediatype, _, err := mime.ParseMediaType(contentType)
		if err != nil {
			return false
		}
		return c.Request().Body == http.NoBody || mediatype == FormDataMimeType || mediatype == URLEncodedMimeType
	}
	return c.Request().Body == http.NoBody
}

func EnforceJSONContentType(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if enforceJSONContentTypeSkipper(c) {
			return next(c)
		}
		mediatype, _, err := mime.ParseMediaType(c.Request().Header.Get("Content-Type"))
		if err != nil {
			err = ce.NewErrorResponse(http.StatusUnsupportedMediaType, "Error parsing content type", err.Error())
			c.Error(err)
			return nil
		}
		if mediatype != JSONMimeType {
			err = ce.NewErrorResponse(http.StatusUnsupportedMediaType, "Incorrect content type", "Content-Type must be application/json")
			c.Error(err)
			return nil
		}
		return next(c)
	}
}
