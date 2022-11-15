package external_repos

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRemoveDuplicates(t *testing.T) {
	assert.Equal(t, []string{"a", "b"}, removeDuplicates([]string{"a", "b", "a"}))
	assert.Equal(t, []string{"a", "b"}, removeDuplicates([]string{"a", "b"}))
}
