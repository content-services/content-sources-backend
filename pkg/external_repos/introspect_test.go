package external_repos

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsRedHatUrl(t *testing.T) {

	assert.True(t, IsRedHat("https://cdn.redhat.com/content/"))
	assert.False(t, IsRedHat("https://someotherdomain.com/myrepo/url"))
}
