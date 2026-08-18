package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePURL_Maven(t *testing.T) {
	pkg := parsePURL("pkg:maven/org.springframework/spring-core@5.3.20?type=jar#some/subpath")
	require.NotNil(t, pkg)
	assert.Equal(t, EcosystemJava, pkg.Ecosystem)
	assert.Equal(t, "spring-core", pkg.Name)
	assert.Equal(t, "5.3.20", pkg.Version)
	assert.Equal(t, "org.springframework", pkg.Namespace)
}

func TestParsePURL_PyPI(t *testing.T) {
	pkg := parsePURL("pkg:pypi/flask@2.3.0")
	require.NotNil(t, pkg)
	assert.Equal(t, EcosystemPython, pkg.Ecosystem)
	assert.Equal(t, "flask", pkg.Name)
	assert.Equal(t, "2.3.0", pkg.Version)
	assert.Empty(t, pkg.Namespace)
}

func TestParsePURL_UnsupportedEcosystem(t *testing.T) {
	assert.Nil(t, parsePURL("pkg:npm/react@18.2.0"))
	assert.Nil(t, parsePURL("pkg:golang/github.com/gin-gonic/gin@1.9.1"))
	assert.Nil(t, parsePURL("pkg:cargo/serde@1.0.0"))
}

func TestParsePURL_Invalid(t *testing.T) {
	assert.Nil(t, parsePURL(""))
	assert.Nil(t, parsePURL("not-a-purl"))
	assert.Nil(t, parsePURL("pkg:"))
	assert.Nil(t, parsePURL("pkg:maven/"))
}
