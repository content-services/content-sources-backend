package middleware

import (
	"bytes"
	"errors"
	"io"
	"net/http"

	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/labstack/echo/v4"
)

const BodyDumpLimit = 1000
const BodyStoreKey = "body_backup"

func LogServerErrorRequest(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) (err error) {
		if c.Get(BodyStoreKey) == nil {
			storeRequestBody(c)
		}
		if err = next(c); err != nil {
			if containsServerError(err) {
				logRequestBody(c)
			}
			return err
		}
		return nil
	}
}

func containsServerError(err error) bool {
	httpError := new(ce.ErrorResponse)
	if errors.As(err, httpError) {
		for _, e := range httpError.Errors {
			if e.Status >= http.StatusInternalServerError {
				return true
			}
		}
	}
	return false
}

func logRequestBody(c echo.Context) {
	if body := c.Get(BodyStoreKey); body != nil {
		storedBodyBytes, ok := body.([]byte)
		if !ok {
			c.Logger().Error("Error reading request body")
		}
		c.Logger().Errorf("Request body: %v", string(storedBodyBytes))
	}
}

func storeRequestBody(c echo.Context) {
	var reqBody []byte
	if c.Request().Body != nil {
		reqBody, _ = io.ReadAll(c.Request().Body)
	}
	c.Request().Body = io.NopCloser(bytes.NewBuffer(reqBody))

	limit := min(len(reqBody), BodyDumpLimit)
	c.Set(BodyStoreKey, reqBody[:limit])
}
