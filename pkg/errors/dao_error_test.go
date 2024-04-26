package errors

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestError(t *testing.T) {
	err := DaoError{
		Message: "error message",
	}
	assert.Equal(t, "error message", err.Error())
	err.Wrap(errors.New("wrapped error"))
	assert.Equal(t, "error message: wrapped error", err.Error())

	err.Wrap(context.Canceled)
	assert.True(t, errors.Is(&err, context.Canceled))
}
