package middleware

import (
	"errors"

	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/labstack/echo/v4"
)

// ExtractStatus is a middlware that sets the response status
//
//	based on the error returned.  This is meant to be used
//	with our own fork of lecho to figure out the proper logging level
//	based on the Error contained within.
func ExtractStatus(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		var err error
		if err = next(c); err != nil {
			httpErr := new(ce.ErrorResponse)
			if errors.As(err, httpErr) {
				largest := 0
				for _, respErr := range httpErr.Errors {
					if respErr.Status > largest {
						largest = respErr.Status
					}
				}
				c.Response().Status = largest
			}
			return err
		}

		return nil
	}
}
