package parser

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseCSV_Basic(t *testing.T) {
	data := []byte(`vulnerability_id,packageurl,component_name,component_version
CVE-2024-22262,pkg:maven/org.springframework/spring-web@6.1.5,Spring Web,6.1.5
CVE-2024-34062,pkg:pypi/requests@2.31.0,Requests,2.31.0
`)

	pkgs, err := parseCSV(bytes.NewReader(data))
	require.NoError(t, err)
	assert.Len(t, pkgs, 2)
	assert.Equal(t, EcosystemJava, pkgs[0].Ecosystem)
	assert.Equal(t, "spring-web", pkgs[0].Name)
	assert.Equal(t, "6.1.5", pkgs[0].Version)
	assert.Equal(t, "org.springframework", pkgs[0].Namespace)
	assert.Equal(t, EcosystemPython, pkgs[1].Ecosystem)
	assert.Equal(t, "requests", pkgs[1].Name)
	assert.Equal(t, "2.31.0", pkgs[1].Version)
	assert.Empty(t, pkgs[1].Namespace)
}

func TestParseCSV_WithMetadataRows(t *testing.T) {
	data := []byte(`Vuln-Report_2026-08-18,,,,
"Some description",,,,
vulnerability_id,packageurl,component_name,component_version
CVE-2024-22262,pkg:maven/org.springframework/spring-web@6.1.5,Spring Web,6.1.5
`)

	pkgs, err := parseCSV(bytes.NewReader(data))
	require.NoError(t, err)
	assert.Len(t, pkgs, 1)
	assert.Equal(t, EcosystemJava, pkgs[0].Ecosystem)
	assert.Equal(t, "spring-web", pkgs[0].Name)
	assert.Equal(t, "6.1.5", pkgs[0].Version)
	assert.Equal(t, "org.springframework", pkgs[0].Namespace)
}

func TestParseCSV_SkipsUnsupportedEcosystems(t *testing.T) {
	data := []byte(`vulnerability_id,packageurl,component_name
CVE-001,pkg:npm/express@4.18.2,Express
CVE-002,pkg:pypi/flask@3.0.3,Flask
`)

	pkgs, err := parseCSV(bytes.NewReader(data))
	require.NoError(t, err)
	assert.Len(t, pkgs, 1)
	assert.Equal(t, EcosystemPython, pkgs[0].Ecosystem)
	assert.Equal(t, "flask", pkgs[0].Name)
	assert.Equal(t, "3.0.3", pkgs[0].Version)
	assert.Empty(t, pkgs[0].Namespace)
}

func TestParseCSV_SkipsRowsWithoutPURL(t *testing.T) {
	data := []byte(`vulnerability_id,packageurl,component_name
CVE-001,,UnknownLib
CVE-002,pkg:pypi/flask@3.0.3,Flask
`)

	pkgs, err := parseCSV(bytes.NewReader(data))
	require.NoError(t, err)
	assert.Len(t, pkgs, 1)
	assert.Equal(t, EcosystemPython, pkgs[0].Ecosystem)
	assert.Equal(t, "flask", pkgs[0].Name)
	assert.Equal(t, "3.0.3", pkgs[0].Version)
	assert.Empty(t, pkgs[0].Namespace)
}
