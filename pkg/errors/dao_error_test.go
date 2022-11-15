package errors

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestError(t *testing.T) {
	err := DaoError{
		Message: "error message",
	}
	assert.Equal(t, "error message", err.Error())
	err.Wrap("wrapped error")
	assert.Equal(t, "wrapped error: error message", err.Error())
}
