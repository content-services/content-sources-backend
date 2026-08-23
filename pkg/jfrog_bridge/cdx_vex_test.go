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

	var doc cdxVEX
	require.NoError(t, json.Unmarshal(data, &doc))

	assert.Equal(t, "CycloneDX", doc.BOMFormat)
	assert.Equal(t, "1.6", doc.SpecVersion)

	// Component fields
	assert.Equal(t, "org.springframework", doc.Metadata.Component.Group)
	assert.Equal(t, "spring-core", doc.Metadata.Component.Name)
	assert.Equal(t, "5.3.18.rhlw-00003", doc.Metadata.Component.Version)
	assert.Equal(t, "pkg:maven/org.springframework/spring-core@5.3.18.rhlw-00003?type=jar",
		doc.Metadata.Component.PURL)

	// compatible-with-1 property
	require.Len(t, doc.Metadata.Component.Properties, 1)
	assert.Equal(t, "compatible-with-1", doc.Metadata.Component.Properties[0].Name)
	assert.Equal(t, "pkg:maven/org.springframework/spring-core@5.3.18",
		doc.Metadata.Component.Properties[0].Value)

	// Vulnerabilities
	require.Len(t, doc.Vulns, 2)
	assert.Equal(t, "CVE-2023-20860", doc.Vulns[0].ID)
	assert.Equal(t, "resolved", doc.Vulns[0].Analysis.State)
	assert.Equal(t, "CVE-2025-41249", doc.Vulns[1].ID)
}
