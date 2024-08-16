package middleware

import (
	"bufio"
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
		if c.Get(BodyStoreKey) == nil && isBodiedMethod(c.Request().Method) {
			storeRequestBody(c)
		}
		if err = next(c); err != nil {
			if containsServerError(err) && isBodiedMethod(c.Request().Method) {
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
	limit := BodyDumpLimit
	buffered := BufferedReadCloser{bufio.NewReader(c.Request().Body), c.Request().Body}

	bytes, err := buffered.Peek(1000)
	if errors.Is(err, io.EOF) {
		limit = len(bytes)
		err = nil
	}
	if errors.Is(err, bufio.ErrBufferFull) {
		err = nil
	}
	if err != nil {
		c.Logger().Error("Error reading request body")
		return
	}

	c.Set(BodyStoreKey, bytes[:limit])
	c.Request().Body = buffered
}

func isBodiedMethod(method string) bool {
	switch method {
	case "GET":
		return false
	case "DELETE":
		return false
	default:
		return true
	}
}
