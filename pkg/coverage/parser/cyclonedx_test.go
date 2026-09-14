package parser

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParse_CycloneDXJSON(t *testing.T) {
	result := parseTestdata(t, "sbom.json", filepath.Join("cyclonedx", "bom.json"))
	assert.Equal(t, FormatCycloneDX, result.InputFormat)
	assert.ElementsMatch(t, []Package{
		{Ecosystem: EcosystemJava, Namespace: "com.example", Name: "my-app", Version: "1.0.0"},
		{Ecosystem: EcosystemJava, Namespace: "org.springframework", Name: "spring-core", Version: "5.3.20"},
		{Ecosystem: EcosystemPython, Name: "flask", Version: "3.0.3"},
		{Ecosystem: EcosystemJavaScript, Name: "express", Version: "4.18.2"},
		{Ecosystem: EcosystemJava, Namespace: "org.apache.commons", Name: "commons-lang3", Version: "3.12.0"},
		{Ecosystem: EcosystemPython, Name: "requests", Version: "2.31.0"},
	}, result.Packages)
}

func TestParse_CycloneDXXML(t *testing.T) {
	result := parseTestdata(t, "bom.xml", filepath.Join("cyclonedx", "tools.cdx.xml"))
	assert.Equal(t, FormatCycloneDX, result.InputFormat)
	assert.ElementsMatch(t, []Package{
		{Ecosystem: EcosystemJava, Namespace: "org.springframework", Name: "spring-web", Version: "6.1.5"},
		{Ecosystem: EcosystemPython, Name: "flask", Version: "3.0.3"},
		{Ecosystem: EcosystemJavaScript, Name: "express", Version: "4.18.2"},
	}, result.Packages)
}
