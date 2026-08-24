package parser

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse_CycloneDXJSON(t *testing.T) {
	result := parseTestdata(t, "sbom.json", filepath.Join("cyclonedx", "bom.json"))
	assert.Equal(t, FormatCycloneDX, result.InputFormat)
	assert.ElementsMatch(t, []Package{
		{Ecosystem: EcosystemJava, Namespace: "com.example", Name: "my-app", Version: "1.0.0"},
		{Ecosystem: EcosystemJava, Namespace: "org.springframework", Name: "spring-core", Version: "5.3.20"},
		{Ecosystem: EcosystemPython, Name: "flask", Version: "3.0.3"},
		{Ecosystem: EcosystemJava, Namespace: "org.apache.commons", Name: "commons-lang3", Version: "3.12.0"},
		{Ecosystem: EcosystemPython, Name: "requests", Version: "2.31.0"},
	}, result.Packages)
}

func TestParse_CycloneDXXML(t *testing.T) {
	result := parseTestdata(t, "bom.xml", filepath.Join("cyclonedx", "tools.cdx.xml"))
	assert.Equal(t, FormatCycloneDX, result.InputFormat)
	require.Len(t, result.Packages, 2)
	assert.Equal(t, EcosystemJava, result.Packages[0].Ecosystem)
	assert.Equal(t, "spring-web", result.Packages[0].Name)
	assert.Equal(t, EcosystemPython, result.Packages[1].Ecosystem)
	assert.Equal(t, "flask", result.Packages[1].Name)
}
