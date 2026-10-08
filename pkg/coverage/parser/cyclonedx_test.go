package parser

import (
	"path/filepath"
	"strings"
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

func TestParseCyclonedxSkippedEntries(t *testing.T) {
	tests := []struct {
		name, filename, content string
		skipped, packages       int
	}{
		{"CycloneDX JSON", "bom.cdx.json", `{"bomFormat":"CycloneDX","components":[{"purl":"pkg:pypi/flask@1"},{"purl":"pkg:pypi/flask@1"},{"name":"missing","components":[{"group":"org.example","name":"lib","version":"1"},{"purl":"invalid"}]}]}`, 2, 2},
		{"CycloneDX XML", "bom.cdx.xml", `<bom xmlns="http://cyclonedx.org/schema/bom/1.4"><components><component><purl>pkg:pypi/flask@1</purl></component><component><name>missing</name><components><component><group>org.example</group><name>lib</name><version>1</version></component><component><purl>invalid</purl></component></components></component></components><metadata><tools><component><name>tool</name></component></tools></metadata></bom>`, 2, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Parse(tt.filename, strings.NewReader(tt.content))
			require.NoError(t, err)
			assert.Equal(t, tt.skipped, result.SkippedEntries)
			assert.Len(t, result.Packages, tt.packages)
		})
	}
}

func TestParseSkippedEntriesLargeSBOM(t *testing.T) {
	components := make([]string, 1500)
	for i := range components {
		components[i] = `{"name":"missing"}`
		if i < 600 {
			components[i] = `{"purl":"pkg:pypi/flask@1"}`
		}
	}
	result, err := Parse("bom.cdx.json", strings.NewReader(`{"bomFormat":"CycloneDX","components":[`+strings.Join(components, ",")+`]}`))
	require.NoError(t, err)
	assert.Equal(t, 900, result.SkippedEntries)
	assert.Len(t, result.Packages, 1) // Valid duplicates are deduplicated, never counted as skipped.
}
