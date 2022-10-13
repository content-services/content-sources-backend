package errors

import (
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
)

type HandlerError struct {
	Status int    `json:"status,omitempty"` // HTTP status code applicable to the error
	Title  string `json:"title,omitempty"`  // A summary of the problem
	Detail string `json:"detail,omitempty"` // An explanation specific to the problem
}

type ErrorResponse struct {
	Errors []HandlerError `json:"errors"`
}

// Error makes it compatible with `error` interface.
func (er HandlerError) Error() string {
	return fmt.Sprintf("code=%d, title=%v, detail=%v", er.Status, er.Title, er.Detail)
}

// ErrorResponse makes it compatible with `error` interface.
func (er ErrorResponse) Error() string {
	var msg string
	for _, err := range er.Errors {
		msg += fmt.Sprintf("error: %s \n", err.Error())
	}
	return msg
}

func NewErrorResponse(code int, title string, detail string) ErrorResponse {
	return ErrorResponse{Errors: []HandlerError{
		{
			Status: code,
			Title:  title,
			Detail: detail,
		}},
	}
}

// NewErrorResponseFromError creates a new ErrorResponse from a list of errors.
func NewErrorResponseFromError(title string, errs ...error) ErrorResponse {
	if len(errs) == 0 {
		return ErrorResponse{}
	}

	errors := make([]HandlerError, len(errs))
	if len(errs) == 1 {
		errors[0] = HandlerError{
			Status: HttpCodeForDaoError(errs[0]),
			Title:  title,
			Detail: errs[0].Error(),
		}
	} else {
		for i := 0; i < len(errs); i++ {
			if errs[i] != nil {
				errors[i] = HandlerError{
					Status: HttpCodeForDaoError(errs[i]),
					Title:  title,
					Detail: errs[i].Error(),
				}
			} else {
				errors[i] = HandlerError{}
			}
		}
	}
	return ErrorResponse{Errors: errors}
}

// NewErrorResponseFromEchoError creates a new ErrorResponse instance from an echo.HTTPError instance
func NewErrorResponseFromEchoError(echoErr *echo.HTTPError) ErrorResponse {
	var detail string
	if m, ok := echoErr.Message.(string); ok {
		detail = m
	} else {
		detail = echoErr.Error()
	}
	return ErrorResponse{Errors: []HandlerError{
		{
			Status: echoErr.Code,
			Title:  "",
			Detail: detail,
		}},
	}
}

// HttpCodeForDaoError returns http code for corresponding dao error
func HttpCodeForDaoError(err error) int {
	daoError, ok := err.(*DaoError)
	if ok {
		if daoError.NotFound {
			return http.StatusNotFound
		} else if daoError.BadValidation {
			return http.StatusBadRequest
		} else {
			return http.StatusInternalServerError
		}
	} else {
		return http.StatusInternalServerError
	}
}

// GetGeneralResponseCode returns the most common error code class in response
func GetGeneralResponseCode(response ErrorResponse) int {
	if len(response.Errors) == 0 {
		return 200
	}

	if len(response.Errors) == 1 {
		return response.Errors[0].Status
	}

	codes := []int{0, 0, 0, 0, 0} // 100, 200, 300, 400, 500

	for _, err := range response.Errors {
		switch code := err.Status; {
		case code >= 100 && code < 200:
			codes[0]++
		case code >= 200 && code < 300 || code == 0:
			codes[1]++
		case code >= 300 && code < 400:
			codes[2]++
		case code >= 400 && code < 500:
			codes[3]++
		default:
			codes[4]++
		}
	}

	var max int
	for i := len(codes) - 1; i >= 0; i-- {
		if codes[i] > 0 {
			max = i
			break
		}
	}

	return (max + 1) * 100
}
