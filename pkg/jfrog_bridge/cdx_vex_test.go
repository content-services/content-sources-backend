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

	sbomSN := "urn:uuid:test-sbom-1234"
	data, err := GenerateCycloneDXVEX("org.springframework", "spring-core",
		"5.3.18.rhlw-00003", "5.3.18", sbomSN, records)
	require.NoError(t, err)

	var doc cdxBOM
	require.NoError(t, json.Unmarshal(data, &doc))

	assert.Equal(t, "CycloneDX", doc.BOMFormat)
	assert.Equal(t, "1.7", doc.SpecVersion)
	assert.Equal(t, 1, doc.Version)
	assert.True(t, len(doc.SerialNumber) > 0)

	// No metadata or components in VEX
	assert.Nil(t, doc.Metadata)
	assert.Empty(t, doc.Components)

	// Vulnerabilities
	require.Len(t, doc.Vulnerabilities, 2)

	vuln0 := doc.Vulnerabilities[0]
	assert.Equal(t, "CVE-2023-20860", vuln0.ID)
	assert.Equal(t, "resolved", vuln0.Analysis.State)
	assert.Equal(t, []string{"update"}, vuln0.Analysis.Response)
	assert.Contains(t, vuln0.Analysis.Detail, "Update to spring-core 5.3.18.rhlw-00003")
	assert.Contains(t, vuln0.Recommendation, "Update to spring-core 5.3.18.rhlw-00003")

	// Affects with URN cross-reference
	require.Len(t, vuln0.Affects, 1)
	assert.Contains(t, vuln0.Affects[0].Ref, "test-sbom-1234")
	require.Len(t, vuln0.Affects[0].Versions, 1)
	assert.Equal(t, "5.3.18.rhlw-00003", vuln0.Affects[0].Versions[0].Version)
	assert.Equal(t, "unaffected", vuln0.Affects[0].Versions[0].Status)

	// References
	require.Len(t, vuln0.References, 1)
	assert.Equal(t, "CVE-2023-20860", vuln0.References[0].ID)
	assert.Contains(t, vuln0.References[0].Source.URL, "nvd.nist.gov")

	// Ratings
	require.Len(t, vuln0.Ratings, 1)
	assert.Equal(t, "Red Hat Lightwell", vuln0.Ratings[0].Source.Name)
	assert.Equal(t, "CVSSv31", vuln0.Ratings[0].Method)
	assert.Equal(t, 7.5, vuln0.Ratings[0].Score)
	assert.Equal(t, "high", vuln0.Ratings[0].Severity)

	// Advisories include NVD entry
	require.GreaterOrEqual(t, len(vuln0.Advisories), 1)
	assert.Contains(t, vuln0.Advisories[0].URL, "nvd.nist.gov")

	// Published/updated timestamps
	assert.NotEmpty(t, vuln0.Published)
	assert.NotEmpty(t, vuln0.Updated)
}

func TestGenerateCycloneDXVEX_ZeroRecords(t *testing.T) {
	data, err := GenerateCycloneDXVEX("org.springframework", "spring-core",
		"5.3.18.rhlw-00003", "5.3.18", "urn:uuid:foo", nil)
	require.NoError(t, err)

	var doc cdxBOM
	require.NoError(t, json.Unmarshal(data, &doc))

	assert.Empty(t, doc.Vulnerabilities)
	assert.Nil(t, doc.Metadata)
}

func TestGenerateCycloneDXSBOM(t *testing.T) {
	records := []OSVRecord{
		{
			CVEID:      "CVE-2022-40152",
			CVSSVector: "CVSS:3.1/AV:N/AC:L/PR:L/UI:N/S:U/C:N/I:N/A:H",
			CVSSScore:  6.5,
			Severity:   "medium",
		},
	}

	sn, data, err := GenerateCycloneDXSBOM(
		"com.fasterxml.woodstox", "woodstox-core",
		"6.0.3.rhlw-00001", "6.0.3",
		"45a6d86df90579c6d6c0377cca25f5c957e327d43f5f36c00b60a99cc89b57ce",
		records,
	)
	require.NoError(t, err)
	assert.True(t, len(sn) > 0)

	var doc cdxBOM
	require.NoError(t, json.Unmarshal(data, &doc))

	assert.Equal(t, "CycloneDX", doc.BOMFormat)
	assert.Equal(t, "1.7", doc.SpecVersion)
	assert.Equal(t, sn, doc.SerialNumber)

	// Metadata has timestamp only
	require.NotNil(t, doc.Metadata)
	assert.NotEmpty(t, doc.Metadata.Timestamp)

	// No vulnerabilities in SBOM
	assert.Empty(t, doc.Vulnerabilities)

	// One component
	require.Len(t, doc.Components, 1)
	comp := doc.Components[0]
	assert.Equal(t, "library", comp.Type)
	assert.Equal(t, "com.fasterxml.woodstox", comp.Group)
	assert.Equal(t, "woodstox-core", comp.Name)
	assert.Equal(t, "6.0.3.rhlw-00001", comp.Version)
	assert.Contains(t, comp.PackageURL, "repository_url=")

	// SHA-256 hash
	require.Len(t, comp.Hashes, 1)
	assert.Equal(t, "SHA-256", comp.Hashes[0].Alg)
	assert.Equal(t, "45a6d86df90579c6d6c0377cca25f5c957e327d43f5f36c00b60a99cc89b57ce", comp.Hashes[0].Content)

	// Pedigree
	require.NotNil(t, comp.Pedigree)
	require.Len(t, comp.Pedigree.Ancestors, 1)
	assert.Equal(t, "6.0.3", comp.Pedigree.Ancestors[0].Version)
	assert.Equal(t, "pkg:maven/com.fasterxml.woodstox/woodstox-core@6.0.3",
		comp.Pedigree.Ancestors[0].PackageURL)

	// Pedigree patches
	require.Len(t, comp.Pedigree.Patches, 1)
	assert.Equal(t, "backport", comp.Pedigree.Patches[0].Type)
	assert.Equal(t, "CVE-2022-40152", comp.Pedigree.Patches[0].Resolves[0].ID)

	// Pedigree notes
	assert.Contains(t, comp.Pedigree.Notes, "Lightwell remediation")
	assert.Contains(t, comp.Pedigree.Notes, "woodstox-core 6.0.3")
}

func TestGenerateCycloneDXSBOM_NoPedigreeWithoutSuffix(t *testing.T) {
	sn, data, err := GenerateCycloneDXSBOM(
		"org.test", "test-lib", "1.0.0", "1.0.0", "abc123", nil,
	)
	require.NoError(t, err)
	assert.NotEmpty(t, sn)

	var doc cdxBOM
	require.NoError(t, json.Unmarshal(data, &doc))

	require.Len(t, doc.Components, 1)
	assert.Nil(t, doc.Components[0].Pedigree)
}

func TestBuildAffectsRef(t *testing.T) {
	ref := buildAffectsRef("urn:uuid:4048efaf-e166-4041-b19f-6a1b2fec9293", 1,
		"pkg:maven/com.fasterxml.woodstox/woodstox-core@6.0.3.rhlw-00001")
	assert.Contains(t, ref, "urn:cdx:4048efaf-e166-4041-b19f-6a1b2fec9293/1#")
	assert.Contains(t, ref, "pkg")
}

func TestGenerateCycloneDXVEX_WithAdvisories(t *testing.T) {
	records := []OSVRecord{
		{
			CVEID:        "CVE-2022-40152",
			CVSSVector:   "CVSS:3.1/AV:N/AC:L/PR:L/UI:N/S:U/C:N/I:N/A:H",
			CVSSScore:    6.5,
			Severity:     "medium",
			IssueURL:     "https://github.com/FasterXML/woodstox/issues/160",
			FixCommitURL: "https://github.com/FasterXML/woodstox/commit/e8f00401",
		},
	}

	data, err := GenerateCycloneDXVEX("com.fasterxml.woodstox", "woodstox-core",
		"6.0.3.rhlw-00001", "6.0.3", "urn:uuid:test", records)
	require.NoError(t, err)

	var doc cdxBOM
	require.NoError(t, json.Unmarshal(data, &doc))

	require.Len(t, doc.Vulnerabilities, 1)
	advisories := doc.Vulnerabilities[0].Advisories
	require.Len(t, advisories, 3)
	assert.Contains(t, advisories[0].URL, "nvd.nist.gov")
	assert.Equal(t, "Upstream issue report", advisories[1].Title)
	assert.Equal(t, "Upstream fix commit", advisories[2].Title)
}
