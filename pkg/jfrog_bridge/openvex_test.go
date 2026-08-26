package jfrog_bridge

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateOpenVEXPredicate(t *testing.T) {
	records := []OSVRecord{
		{
			CVEID:       "CVE-2023-20860",
			Description: "Spring mvcRequestMatcher issue",
			CVSSVector:  "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:N/I:H/A:N",
			CVSSScore:   7.5,
			Severity:    "high",
		},
		{
			CVEID:       "CVE-2025-41249",
			Description: "Spring expression DoS",
			CVSSVector:  "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:N/I:N/A:H",
			CVSSScore:   7.5,
			Severity:    "high",
		},
	}

	data, err := GenerateOpenVEXPredicate("org.springframework", "spring-core", "5.3.18.rhlw-00003", records)
	require.NoError(t, err)

	var doc map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &doc))

	assert.Equal(t, "https://openvex.dev/ns/v0.2.0", doc["@context"])

	stmts, ok := doc["statements"].([]interface{})
	require.True(t, ok)
	assert.Len(t, stmts, 2)

	expectedPURL := "pkg:maven/org.springframework/spring-core@5.3.18.rhlw-00003?type=jar"
	expectedCVEs := []string{"CVE-2023-20860", "CVE-2025-41249"}

	for i, s := range stmts {
		stmt, ok := s.(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "fixed", stmt["status"])

		vuln, ok := stmt["vulnerability"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, expectedCVEs[i], vuln["name"])

		products, ok := stmt["products"].([]interface{})
		require.True(t, ok)
		require.Len(t, products, 1)
		prod, ok := products[0].(map[string]interface{})
		require.True(t, ok)
		identifiers, ok := prod["identifiers"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, expectedPURL, identifiers["purl"])
	}
}
