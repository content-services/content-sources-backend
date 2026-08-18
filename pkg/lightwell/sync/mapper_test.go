package sync

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/clients/jira_client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMapVulnerability(t *testing.T) {
	issue := validJiraIssue("LTWL-1")
	issue.Fields["description"] = json.RawMessage(`{
		"type":"doc","content":[
			{"type":"paragraph","content":[{"type":"text","text":"Title:","marks":[{"type":"strong"}]},{"type":"text","text":" Example vulnerability"}]},
			{"type":"paragraph","content":[{"type":"text","text":"Component: example-component"}]},
			{"type":"paragraph","content":[{"type":"text","text":"version: 1.2.3"}]},
			{"type":"paragraph","content":[{"type":"text","text":"Details"},{"type":"hardBreak"},{"type":"text","text":"second line"}]}
		]}`)
	issue.Fields[fieldPURL] = json.RawMessage(`"pkg:maven/example@example"`)
	issue.Fields[fieldCWE] = json.RawMessage(`["CWE-79",{"value":"CWE-89"},"CWE-79"]`)
	issue.Fields[fieldSeverity] = json.RawMessage(`{"id":"19917"}`)
	issue.Fields[fieldCVSS] = json.RawMessage(`"CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H"`)
	issue.Fields[fieldEmbargo] = json.RawMessage(`true`)
	issue.Fields["labels"] = json.RawMessage(`["backend","java"]`)

	vulnerability, err := mapVulnerability(issue)
	require.NoError(t, err)
	assert.Equal(t, "LTWL-1:LW-0000-0001", vulnerability.VulnerabilityID)
	assert.Equal(t, "Example vulnerability", *vulnerability.Title)
	assert.Equal(t, "example-component", vulnerability.ComponentName)
	assert.Equal(t, "1.2.3", vulnerability.ComponentVersion)
	assert.Equal(t, "pkg:maven/example@example", *vulnerability.PURL)
	assert.Equal(t, "CWE-79,CWE-89", *vulnerability.CWE)
	assert.Equal(t, "Critical", vulnerability.Severity)
	assert.Equal(t, 9.8, *vulnerability.CVSS)
	assert.NotNil(t, vulnerability.CVSSVector)
	assert.Equal(t, "Submitted", vulnerability.Stage)
	assert.Equal(t, "java", *vulnerability.Language)
	assert.True(t, vulnerability.Embargo)
	assert.Equal(t, "Title: Example vulnerability\nComponent: example-component\nversion: 1.2.3\nDetails\nsecond line", *vulnerability.Description)
	assert.Equal(t, time.Date(2026, 8, 16, 1, 30, 0, 0, time.UTC), vulnerability.SubmittedDate)
	assert.Equal(t, time.Date(2026, 8, 17, 1, 40, 0, 0, time.UTC), vulnerability.LastUpdated)
}

func TestMapVulnerabilitySkipsMalformedOptionalFields(t *testing.T) {
	issue := validJiraIssue("LTWL-2")
	issue.Fields["description"] = json.RawMessage(`{"broken"`)
	issue.Fields[fieldCWE] = json.RawMessage(`{"unexpected":true}`)
	issue.Fields[fieldCVSS] = json.RawMessage(`"not-cvss"`)
	issue.Fields[fieldSeverity] = json.RawMessage(`{"id":"unknown"}`)
	issue.Fields[fieldEmbargo] = json.RawMessage(`"not-a-boolean"`)

	vulnerability, err := mapVulnerability(issue)
	require.NoError(t, err)
	assert.Nil(t, vulnerability.Title)
	assert.Empty(t, vulnerability.ComponentName)
	assert.Nil(t, vulnerability.CWE)
	assert.Nil(t, vulnerability.CVSS)
	assert.Nil(t, vulnerability.CVSSVector)
	assert.Empty(t, vulnerability.Severity)
	assert.False(t, vulnerability.Embargo)
}

func TestMapVulnerabilityUsesCVEFromSummary(t *testing.T) {
	issue := validJiraIssue("LTWL-4")
	issue.Fields["summary"] = json.RawMessage(`"CVE-0000-0001 Example product"`)

	vulnerability, err := mapVulnerability(issue)
	require.NoError(t, err)
	assert.Equal(t, "LTWL-4:CVE-0000-0001", vulnerability.VulnerabilityID)
}

func TestMapVulnerabilityRejectsMissingVulnerabilityID(t *testing.T) {
	issue := validJiraIssue("LTWL-5")
	issue.Fields["summary"] = json.RawMessage(`"Example product without an id"`)

	_, err := mapVulnerability(issue)
	assert.ErrorContains(t, err, "vulnerability id")
}

func TestMapVulnerabilityRejectsMissingTimestamp(t *testing.T) {
	issue := validJiraIssue("LTWL-3")
	delete(issue.Fields, "updated")

	_, err := mapVulnerability(issue)
	assert.ErrorContains(t, err, "updated timestamp")
}

func TestParseCVSSNumeric(t *testing.T) {
	score, vector := parseCVSS(json.RawMessage(`"7.5"`))
	require.NotNil(t, score)
	assert.Equal(t, 7.5, *score)
	assert.Nil(t, vector)
}

func TestLinkedIssueKeyUsesDirectionLabel(t *testing.T) {
	issue := jira_client.JiraIssue{Fields: map[string]json.RawMessage{
		"issuelinks": json.RawMessage(`[
			{"type":{"inward":"is related to","outward":"relates from"},"inwardIssue":{"key":"BATCH-1"}},
			{"type":{"inward":"other","outward":"is related to"},"outwardIssue":{"key":"BATCH-2"}}
		]`),
	}}
	assert.Equal(t, "BATCH-1", linkedIssueKey(issue, "is related to"))
}

func validJiraIssue(key string) jira_client.JiraIssue {
	return jira_client.JiraIssue{Key: key, Fields: map[string]json.RawMessage{
		"summary": json.RawMessage(`"LW-0000-0001 Example component"`),
		"created": json.RawMessage(`"2026-08-15T23:30:00-0200"`),
		"updated": json.RawMessage(`"2026-08-16T23:40:00-0200"`),
		"status":  json.RawMessage(`{"name":"New"}`),
	}}
}
