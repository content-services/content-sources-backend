package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRequirements_VersionOperators(t *testing.T) {
	data := []byte("django==5.0.4\nrequests>=2.31.0\nscipy~=1.13\nnumpy<=1.26.4\n")

	pkgs, err := parseRequirements(data)
	require.NoError(t, err)
	assert.Len(t, pkgs, 4)
	assert.Equal(t, EcosystemPython, pkgs[0].Ecosystem)
	assert.Equal(t, "django", pkgs[0].Name)
	assert.Equal(t, "5.0.4", pkgs[0].Version)
	assert.Empty(t, pkgs[0].Namespace)
	assert.Equal(t, "requests", pkgs[1].Name)
	assert.Empty(t, pkgs[1].Version)
	assert.Empty(t, pkgs[1].Namespace)
	assert.Equal(t, "scipy", pkgs[2].Name)
	assert.Empty(t, pkgs[2].Version)
	assert.Empty(t, pkgs[2].Namespace)
	assert.Equal(t, "numpy", pkgs[3].Name)
	assert.Empty(t, pkgs[3].Version)
	assert.Empty(t, pkgs[3].Namespace)
}

func TestParseRequirements_SkipsFlags(t *testing.T) {
	data := []byte("# comment line\n-r other.txt\n--index-url https://pypi.org/simple\nflask==2.3.0  # web framework\n")

	pkgs, err := parseRequirements(data)
	require.NoError(t, err)
	assert.Len(t, pkgs, 1)
	assert.Equal(t, EcosystemPython, pkgs[0].Ecosystem)
	assert.Equal(t, "flask", pkgs[0].Name)
	assert.Equal(t, "2.3.0", pkgs[0].Version)
	assert.Empty(t, pkgs[0].Namespace)
}

func TestParseRequirements_EnvironmentMarkers(t *testing.T) {
	data := []byte("requests==2.31.0 ; python_version >= '3.8'\n")

	pkgs, err := parseRequirements(data)
	require.NoError(t, err)
	assert.Len(t, pkgs, 1)
	assert.Equal(t, EcosystemPython, pkgs[0].Ecosystem)
	assert.Equal(t, "requests", pkgs[0].Name)
	assert.Equal(t, "2.31.0", pkgs[0].Version)
	assert.Empty(t, pkgs[0].Namespace)
}

func TestParseRequirements_Extras(t *testing.T) {
	data := []byte("requests[security]==2.31.0\nurllib3[socks]==2.0.4\nmatplotlib[security==1.0.0\n")

	pkgs, err := parseRequirements(data)
	require.NoError(t, err)
	assert.Len(t, pkgs, 3)
	assert.Equal(t, EcosystemPython, pkgs[0].Ecosystem)
	assert.Equal(t, "requests", pkgs[0].Name)
	assert.Equal(t, "2.31.0", pkgs[0].Version)
	assert.Empty(t, pkgs[0].Namespace)
	assert.Equal(t, EcosystemPython, pkgs[1].Ecosystem)
	assert.Equal(t, "urllib3", pkgs[1].Name)
	assert.Equal(t, "2.0.4", pkgs[1].Version)
	assert.Empty(t, pkgs[1].Namespace)
	assert.Equal(t, EcosystemPython, pkgs[2].Ecosystem)
	assert.Equal(t, "matplotlib", pkgs[2].Name)
	assert.Empty(t, pkgs[2].Version)
	assert.Empty(t, pkgs[2].Namespace)
}

func TestParseRequirements_LineContinuation(t *testing.T) {
	data := []byte("requests==2.31.0 \\\n    --hash=sha256:abc123\nflask==2.3.0\n")

	pkgs, err := parseRequirements(data)
	require.NoError(t, err)
	assert.Len(t, pkgs, 2)
	assert.Equal(t, EcosystemPython, pkgs[0].Ecosystem)
	assert.Equal(t, "requests", pkgs[0].Name)
	assert.Equal(t, "2.31.0", pkgs[0].Version)
	assert.Empty(t, pkgs[0].Namespace)
	assert.Equal(t, EcosystemPython, pkgs[1].Ecosystem)
	assert.Equal(t, "flask", pkgs[1].Name)
	assert.Equal(t, "2.3.0", pkgs[1].Version)
	assert.Empty(t, pkgs[1].Namespace)
}

func TestParseRequirements_DirectURL(t *testing.T) {
	data := []byte("urllib3 @ https://github.com/urllib3/urllib3/archive/refs/tags/1.26.8.zip\nflask==2.3.0\n")

	pkgs, err := parseRequirements(data)
	require.NoError(t, err)
	assert.Len(t, pkgs, 2)
	assert.Equal(t, EcosystemPython, pkgs[0].Ecosystem)
	assert.Equal(t, "urllib3", pkgs[0].Name)
	assert.Empty(t, pkgs[0].Version)
	assert.Empty(t, pkgs[0].Namespace)
	assert.Equal(t, EcosystemPython, pkgs[1].Ecosystem)
	assert.Equal(t, "flask", pkgs[1].Name)
	assert.Equal(t, "2.3.0", pkgs[1].Version)
	assert.Empty(t, pkgs[1].Namespace)
}

func TestParseRequirements_CommaVersionSpecifiers(t *testing.T) {
	data := []byte("pydantic>=2.0,<3.0,!=2.5.0\ncelery>=5.3.0,<6.0\n")

	pkgs, err := parseRequirements(data)
	require.NoError(t, err)
	assert.Len(t, pkgs, 2)
	assert.Equal(t, EcosystemPython, pkgs[0].Ecosystem)
	assert.Equal(t, "pydantic", pkgs[0].Name)
	assert.Empty(t, pkgs[0].Version)
	assert.Empty(t, pkgs[0].Namespace)
	assert.Equal(t, "celery", pkgs[1].Name)
	assert.Empty(t, pkgs[1].Version)
	assert.Empty(t, pkgs[1].Namespace)
}

func TestParseRequirements_TrailingContinuationError(t *testing.T) {
	data := []byte("flask==2.3.0\nrequests==2.31.0 \\")

	_, err := parseRequirements(data)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected end of file")
}
