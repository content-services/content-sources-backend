package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLightwellTokenAllows(t *testing.T) {
	assert.True(t, LightwellTokenAllows(LightwellAccessValidated, LightwellAccessValidated))
	assert.True(t, LightwellTokenAllows(LightwellAccessValidated, LightwellAccessRemediated))
	assert.False(t, LightwellTokenAllows(LightwellAccessRemediated, LightwellAccessValidated))
	assert.True(t, LightwellTokenAllows(LightwellAccessRemediated, LightwellAccessRemediated))
	assert.False(t, LightwellTokenAllows("unknown", LightwellAccessValidated))
	assert.False(t, LightwellTokenAllows(LightwellAccessValidated, "unknown"))
}

func TestSecurityLevelFromContentPath(t *testing.T) {
	assert.Equal(t, LightwellAccessValidated,
		SecurityLevelFromContentPath("/api/pulp-content/lightwell/java/validated/junit/junit/4.13.2/junit-4.13.2.pom"))
	assert.Equal(t, LightwellAccessRemediated,
		SecurityLevelFromContentPath("java/remediated/foo"))
	assert.Equal(t, "", SecurityLevelFromContentPath("/api/pulp-content/lightwell/java/other/pkg"))
	assert.Equal(t, "", SecurityLevelFromContentPath(""))
}

func TestValidLightwellAccessLevel(t *testing.T) {
	assert.True(t, ValidLightwellAccessLevel(LightwellAccessValidated))
	assert.True(t, ValidLightwellAccessLevel(LightwellAccessRemediated))
	assert.False(t, ValidLightwellAccessLevel(""))
	assert.False(t, ValidLightwellAccessLevel("novel"))
}
