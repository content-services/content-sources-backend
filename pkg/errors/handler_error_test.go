package errors

import (
	"errors"
	"net/http"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

func TestNewErrorResponse(t *testing.T) {
	expected := ErrorResponse{Errors: []HandlerError{
		{
			Status: http.StatusBadRequest,
			Title:  "title",
			Detail: "detail",
		},
	}}
	result := NewErrorResponse(http.StatusBadRequest, "title", "detail")
	assert.Equal(t, expected, result)
}

func TestNewErrorResponseFromError(t *testing.T) {
	// Test no errors
	expected := ErrorResponse{}
	result := NewErrorResponseFromError("")
	assert.Equal(t, expected, result)

	// Test one error
	expected = ErrorResponse{Errors: []HandlerError{
		{
			Status: http.StatusInternalServerError,
			Title:  "an error's title",
			Detail: "an unexpected error",
		},
	}}
	errs := []error{
		errors.New("an unexpected error"),
	}
	result = NewErrorResponseFromError("an error's title", errs...)
	assert.Equal(t, expected, result)

	// Test list of errors
	expected = ErrorResponse{Errors: []HandlerError{
		{
			Status: http.StatusNotFound,
			Title:  "an error's title",
			Detail: "not found",
		},
		{
			Status: http.StatusBadRequest,
			Title:  "an error's title",
			Detail: "bad validation",
		},
		{
			Status: http.StatusInternalServerError,
			Title:  "an error's title",
			Detail: "unknown error",
		},
		{},
	}}
	daoErrs := []error{
		&DaoError{
			Message:       "not found",
			NotFound:      true,
			BadValidation: false,
		},
		&DaoError{
			Message:       "bad validation",
			NotFound:      false,
			BadValidation: true,
		},
		&DaoError{
			Message:       "unknown error",
			NotFound:      false,
			BadValidation: false,
		},
		nil,
	}
	result = NewErrorResponseFromError("an error's title", daoErrs...)
	msg := result.Error()
	assert.Equal(t, expected, result)
	assert.Equal(t, "code=404, title=an error's title, detail=not found", expected.Errors[0].Error())
	assert.Contains(t, msg, expected.Errors[0].Error())
	assert.Contains(t, msg, expected.Errors[1].Error())
	assert.Contains(t, msg, expected.Errors[2].Error())
}

func TestNewErrorResponseFromEchoError(t *testing.T) {
	echoErr := echo.NewHTTPError(http.StatusBadRequest, http.StatusText(http.StatusBadRequest))
	echoErrNoMessage := echo.NewHTTPError(http.StatusBadRequest)

	expected := ErrorResponse{Errors: []HandlerError{
		{
			Status: http.StatusBadRequest,
			Title:  "",
			Detail: http.StatusText(http.StatusBadRequest),
		}},
	}

	result := NewErrorResponseFromEchoError(echoErr)
	resultNoMessage := NewErrorResponseFromEchoError(echoErrNoMessage)
	assert.Equal(t, expected, result)
	assert.Equal(t, expected, resultNoMessage)
}

func TestNewErrorResponseFromEchoErrorOtherType(t *testing.T) {
	type otherstring string
	echoErr := echo.NewHTTPError(http.StatusBadRequest, otherstring(http.StatusText(http.StatusBadRequest)))
	expected := ErrorResponse{
		Errors: []HandlerError{
			{
				Status: http.StatusBadRequest,
				Title:  "",
				Detail: "code=400, message=Bad Request",
			},
		},
	}
	result := NewErrorResponseFromEchoError(echoErr)
	assert.Equal(t, expected, result)
}

func TestGetGeneralResponseCode(t *testing.T) {
	// Test no errors
	result := GetGeneralResponseCode(ErrorResponse{})
	assert.Equal(t, 200, result)

	// Test one error
	result = GetGeneralResponseCode(ErrorResponse{Errors: []HandlerError{
		{
			Status: http.StatusBadRequest,
		},
	}})
	assert.Equal(t, 400, result)

	// Test 400
	er := ErrorResponse{Errors: []HandlerError{
		{
			Status: http.StatusBadRequest,
		},
		{
			Status: http.StatusNotFound,
		},
	}}
	result = GetGeneralResponseCode(er)
	assert.Equal(t, 400, result)

	// Test 500
	er = ErrorResponse{Errors: []HandlerError{
		{
			Status: http.StatusContinue,
		},
		{
			Status: http.StatusOK,
		},
		{
			Status: http.StatusFound,
		},
		{
			Status: http.StatusBadRequest,
		},
		{
			Status: http.StatusNotFound,
		},
		{
			Status: http.StatusInternalServerError,
		},
		{
			Status: http.StatusBadGateway,
		},
	}}
	result = GetGeneralResponseCode(er)
	assert.Equal(t, 500, result)
}
