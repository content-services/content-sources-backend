package jfrog_bridge

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateCycloneDXVEX(t *testing.T) {
	records := []OSVRecord{
		{
			CVEID:       "CVE-2023-20860",
			Description: "Security bypass",
			CVSSVector:  "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:N/I:H/A:N",
			CVSSScore:   7.5,
			Severity:    "high",
		},
		{
			CVEID:       "CVE-2025-41249",
			Description: "Annotation bypass",
			CVSSVector:  "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:N/I:H/A:N",
			CVSSScore:   7.5,
			Severity:    "high",
		},
	}

	data, err := GenerateCycloneDXVEX("org.springframework", "spring-core",
		"5.3.18.rhlw-00003", "5.3.18", records)
	require.NoError(t, err)

	var doc cdxBOM
	require.NoError(t, json.Unmarshal(data, &doc))

	assert.Equal(t, "CycloneDX", doc.BOMFormat)
	assert.Equal(t, "1.6", doc.SpecVersion)

	// Metadata component
	assert.Equal(t, "org.springframework", doc.Metadata.Component.Group)
	assert.Equal(t, "spring-core", doc.Metadata.Component.Name)
	assert.Equal(t, "5.3.18.rhlw-00003", doc.Metadata.Component.Version)
	assert.Equal(t, "pkg:maven/org.springframework/spring-core@5.3.18.rhlw-00003?type=jar",
		doc.Metadata.Component.PackageURL)

	// compatible-with-1 property
	require.Len(t, doc.Metadata.Component.Properties, 1)
	assert.Equal(t, "compatible-with-1", doc.Metadata.Component.Properties[0].Name)
	assert.Equal(t, "pkg:maven/org.springframework/spring-core@5.3.18",
		doc.Metadata.Component.Properties[0].Value)

	// metadata.component must NOT have pedigree
	assert.Nil(t, doc.Metadata.Component.Pedigree)

	// Vulnerabilities
	require.Len(t, doc.Vulnerabilities, 2)
	assert.Equal(t, "CVE-2023-20860", doc.Vulnerabilities[0].ID)
	assert.Equal(t, "resolved", doc.Vulnerabilities[0].Analysis.State)
	assert.Equal(t, "CVE-2025-41249", doc.Vulnerabilities[1].ID)

	// Components: one entry with pedigree
	require.Len(t, doc.Components, 1)
	comp := doc.Components[0]
	assert.Equal(t, "library", comp.Type)
	assert.Equal(t, "org.springframework", comp.Group)
	assert.Equal(t, "spring-core", comp.Name)
	assert.Equal(t, "5.3.18.rhlw-00003", comp.Version)

	require.NotNil(t, comp.Pedigree)

	// Pedigree ancestors
	require.Len(t, comp.Pedigree.Ancestors, 1)
	ancestor := comp.Pedigree.Ancestors[0]
	assert.Equal(t, "library", ancestor.Type)
	assert.Equal(t, "org.springframework", ancestor.Group)
	assert.Equal(t, "spring-core", ancestor.Name)
	assert.Equal(t, "5.3.18", ancestor.Version)
	assert.Equal(t, "pkg:maven/org.springframework/spring-core@5.3.18?type=jar", ancestor.PackageURL)

	// Pedigree patches
	require.Len(t, comp.Pedigree.Patches, 2)
	assert.Equal(t, "backport", comp.Pedigree.Patches[0].Type)
	require.Len(t, comp.Pedigree.Patches[0].Resolves, 1)
	assert.Equal(t, "security", comp.Pedigree.Patches[0].Resolves[0].Type)
	assert.Equal(t, "CVE-2023-20860", comp.Pedigree.Patches[0].Resolves[0].ID)
	assert.Equal(t, "NVD", comp.Pedigree.Patches[0].Resolves[0].Source.Name)
	assert.Equal(t, "CVE-2025-41249", comp.Pedigree.Patches[1].Resolves[0].ID)

	// Pedigree notes
	assert.Equal(t, "Backported security fixes to spring-core 5.3.18. Build 5.3.18.rhlw-00003.",
		comp.Pedigree.Notes)
}

func TestGenerateCycloneDXVEX_NoPedigreeWithoutSuffix(t *testing.T) {
	records := []OSVRecord{
		{
			CVEID:       "CVE-2023-20860",
			Description: "Security bypass",
			CVSSVector:  "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:N/I:H/A:N",
			CVSSScore:   7.5,
			Severity:    "high",
		},
	}

	data, err := GenerateCycloneDXVEX("org.springframework", "spring-core",
		"5.3.18", "5.3.18", records)
	require.NoError(t, err)

	var doc cdxBOM
	require.NoError(t, json.Unmarshal(data, &doc))

	assert.Empty(t, doc.Components, "components must be omitted when version has no .rhlw-* suffix")
}

func TestGenerateCycloneDXVEX_ZeroRecords(t *testing.T) {
	data, err := GenerateCycloneDXVEX("org.springframework", "spring-core",
		"5.3.18.rhlw-00003", "5.3.18", nil)
	require.NoError(t, err)

	var doc cdxBOM
	require.NoError(t, json.Unmarshal(data, &doc))

	// Pedigree present with ancestor but no patches
	require.Len(t, doc.Components, 1)
	require.NotNil(t, doc.Components[0].Pedigree)
	require.Len(t, doc.Components[0].Pedigree.Ancestors, 1)
	assert.Empty(t, doc.Components[0].Pedigree.Patches)

	// Vulnerabilities array must still be present (empty)
	assert.NotNil(t, doc.Vulnerabilities)
	assert.Empty(t, doc.Vulnerabilities)
}

func TestGenerateCycloneDXVEX_VulnerabilitiesAlwaysPresent(t *testing.T) {
	data, err := GenerateCycloneDXVEX("org.test", "test-lib",
		"1.0.rhlw-00001", "1.0", nil)
	require.NoError(t, err)

	// Raw JSON must contain the "vulnerabilities" key even when empty
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &raw))
	_, hasVulns := raw["vulnerabilities"]
	assert.True(t, hasVulns, `"vulnerabilities" key must appear in JSON output`)
}
