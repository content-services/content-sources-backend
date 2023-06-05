package notifications

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSetEmptyToNil(t *testing.T) {
	testedVariables := []string{"someValue", "otherValue", "s"}

	for i := 0; i < len(testedVariables); i++ {
		result := SetEmptyToNil(testedVariables[i])
		assert.NotNil(t, result)
	}

	testedNilVariables := []string{""}

	for i := 0; i < len(testedNilVariables); i++ {
		result := SetEmptyToNil(testedNilVariables[i])
		assert.Nil(t, result)
	}
}
