package middleware

import (
	"bufio"
	"io"

	"github.com/labstack/echo/v4"
)

// See: https://github.com/labstack/echo/pull/1502/files
// This method exist for v5 echo framework
func MatchedRoute(ctx echo.Context) string {
	pathx := ctx.Path()
	for _, r := range ctx.Echo().Routes() {
		if pathx == r.Path {
			return r.Path
		}
	}
	return ""
}

type BufferedReadCloser struct {
	*bufio.Reader
	io.ReadCloser
}

func (rw BufferedReadCloser) Close() error {
	return rw.ReadCloser.Close()
}

func (rw BufferedReadCloser) Read(p []byte) (int, error) {
	return rw.Reader.Read(p)
}
