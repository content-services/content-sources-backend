package sync

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/clients/jira_client"
	"github.com/content-services/content-sources-backend/pkg/utils"
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
	issue.Fields[fieldPURL] = json.RawMessage(`"pkg:maven/com.example/purl-component@9.9.9"`)
	issue.Fields[fieldCWE] = json.RawMessage(`["CWE-79",{"value":"CWE-89"},"CWE-79"]`)
	issue.Fields[fieldSeverity] = json.RawMessage(`{"id":"19917"}`)
	issue.Fields[fieldCVSS] = json.RawMessage(`"CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H"`)
	issue.Fields[fieldEmbargo] = json.RawMessage(`true`)
	issue.Fields["labels"] = json.RawMessage(`["backend","java"]`)

	vulnerability, err := mapVulnerability(issue)
	require.NoError(t, err)
	assert.Equal(t, "LTWL-1", vulnerability.VulnerabilityKey)
	assert.Equal(t, "LW-0000-0001", vulnerability.VulnerabilityID)
	assert.Equal(t, "Example vulnerability", *vulnerability.Title)
	assert.Equal(t, "com.example:purl-component", vulnerability.ComponentName)
	assert.Equal(t, "9.9.9", vulnerability.ComponentVersion)
	assert.Equal(t, "pkg:maven/com.example/purl-component@9.9.9", *vulnerability.PURL)
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
	assert.Equal(t, "Example component", *vulnerability.Title)
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
	assert.Equal(t, "LTWL-4", vulnerability.VulnerabilityKey)
	assert.Equal(t, "CVE-0000-0001", vulnerability.VulnerabilityID)
	assert.Equal(t, "Example product", *vulnerability.Title)
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

func TestDiscardedResolution(t *testing.T) {
	assert.True(t, discardedResolution(json.RawMessage(`{"name":"Not a bug"}`)))
	assert.True(t, discardedResolution(json.RawMessage(`{"name":"Duplicate"}`)))
	assert.True(t, discardedResolution(json.RawMessage(`{"name":"Won't do"}`)))
	assert.True(t, discardedResolution(json.RawMessage(`{"name":" won't do "}`)))
	assert.False(t, discardedResolution(json.RawMessage(`{"name":"Done"}`)))
	assert.False(t, discardedResolution(json.RawMessage(`null`)))
	assert.False(t, discardedResolution(nil))
}

func TestLanguagePrefersLabelOverPURL(t *testing.T) {
	purl := utils.ParsePURL("pkg:pypi/flask@3.0.3")
	got := language(json.RawMessage(`["backend","javascript"]`), purl)
	require.NotNil(t, got)
	assert.Equal(t, "javascript", *got)
}

func TestLanguageFromPURL(t *testing.T) {
	npm := utils.ParsePURL("pkg:npm/react@18.2.0")
	maven := utils.ParsePURL("pkg:maven/org.springframework/spring-core@5.3.20")

	assert.Equal(t, "javascript", *language(json.RawMessage(`["backend"]`), npm))
	assert.Equal(t, "java", *language(nil, maven))
	assert.Nil(t, language(nil, utils.ParsePURL("not-a-purl")))
	assert.Nil(t, language(json.RawMessage(`["backend"]`), nil))
}

func TestComponentPrefersPURLOverDescription(t *testing.T) {
	purl := utils.ParsePURL("pkg:pypi/flask@3.0.3")
	assert.Equal(t, "flask", componentName("from-description", purl))
	assert.Equal(t, "3.0.3", componentVersion("9.9.9", purl))
}

func TestComponentFallsBackToDescription(t *testing.T) {
	assert.Equal(t, "from-description", componentName("from-description", nil))
	assert.Equal(t, "9.9.9", componentVersion("9.9.9", nil))

	purlWithoutVersion := utils.ParsePURL("pkg:maven/com.example/component")
	assert.Equal(t, "9.9.9", componentVersion("9.9.9", purlWithoutVersion))
}

func TestComponentFromPURL(t *testing.T) {
	npm := utils.ParsePURL("pkg:npm/react@18.2.0")
	scoped := utils.ParsePURL("pkg:npm/%40angular/core@15.0.0")
	maven := utils.ParsePURL("pkg:maven/org.springframework/spring-core@5.3.20?type=jar")
	pypi := utils.ParsePURL("pkg:pypi/flask@3.0.3")

	assert.Equal(t, "react", componentName("", npm))
	assert.Equal(t, "18.2.0", componentVersion("", npm))
	assert.Equal(t, "@angular/core", componentName("", scoped))
	assert.Equal(t, "org.springframework:spring-core", componentName("", maven))
	assert.Equal(t, "5.3.20", componentVersion("", maven))
	assert.Equal(t, "flask", componentName("", pypi))
	assert.Equal(t, "3.0.3", componentVersion("", pypi))
	assert.Empty(t, componentName("", nil))
	assert.Empty(t, componentVersion("", nil))
	assert.Empty(t, componentName("", utils.ParsePURL("not-a-purl")))
	assert.Empty(t, componentVersion("", utils.ParsePURL("not-a-purl")))
}

func TestPackageURLPrefersCustomFieldOverDescription(t *testing.T) {
	got := packageURL(json.RawMessage(`"pkg:maven/from-field@1"`), "PURL: pkg:pypi/flask@3.0.3")
	require.NotNil(t, got)
	assert.Equal(t, "pkg:maven/from-field@1", *got)
}

func TestPackageURLFromDescription(t *testing.T) {
	got := packageURL(nil, "Title: ignored\nPURL: pkg:npm/react@18.2.0\n")
	require.NotNil(t, got)
	assert.Equal(t, "pkg:npm/react@18.2.0", *got)
	assert.Nil(t, packageURL(nil, "no purl here"))
}

func TestMapVulnerabilityUsesDescriptionPURLWhenFieldMissing(t *testing.T) {
	issue := validJiraIssue("LTWL-7")
	issue.Fields["description"] = json.RawMessage(`{
		"type":"doc","content":[
			{"type":"paragraph","content":[{"type":"text","text":"PURL: pkg:pypi/flask@3.0.3"}]}
		]}`)

	vulnerability, err := mapVulnerability(issue)
	require.NoError(t, err)
	assert.Equal(t, "pkg:pypi/flask@3.0.3", *vulnerability.PURL)
	assert.Equal(t, "flask", vulnerability.ComponentName)
	assert.Equal(t, "3.0.3", vulnerability.ComponentVersion)
	assert.Equal(t, "python", *vulnerability.Language)
}

func TestMapVulnerabilityUsesPURLWhenDescriptionOmitsComponent(t *testing.T) {
	issue := validJiraIssue("LTWL-6")
	issue.Fields[fieldPURL] = json.RawMessage(`"pkg:maven/com.example/demo-lib@1.2.3"`)

	vulnerability, err := mapVulnerability(issue)
	require.NoError(t, err)
	assert.Equal(t, "com.example:demo-lib", vulnerability.ComponentName)
	assert.Equal(t, "1.2.3", vulnerability.ComponentVersion)
	assert.Equal(t, "java", *vulnerability.Language)
}

func TestTitlePrefersDescriptionOverSummary(t *testing.T) {
	assert.Equal(t, "from description", title("Title: from description\n", "LW-0000-0001 Example component"))
}

func TestTitleFromSummaryWithoutVulnerabilityPrefix(t *testing.T) {
	assert.Equal(t, "Example component", title("", "LW-0000-0001 Example component"))
	assert.Equal(t, "Example product", title("no title line", "CVE-0000-0001 Example product"))
	assert.Empty(t, title("", "LW-0000-0001"))
}

func TestSeverityPrefersCustomFieldOverLabel(t *testing.T) {
	assert.Equal(t, "Critical", severity(json.RawMessage(`{"id":"19917"}`), json.RawMessage(`["severity::MEDIUM"]`)))
}

func TestSeverityFromLabelWhenFieldMissing(t *testing.T) {
	assert.Equal(t, "Moderate", severity(nil, json.RawMessage(`["backend","severity::MEDIUM"]`)))
	assert.Equal(t, "Moderate", severity(json.RawMessage(`{"id":"unknown"}`), json.RawMessage(`["severity::medium"]`)))
	assert.Equal(t, "Critical", severity(nil, json.RawMessage(`["severity::CRITICAL"]`)))
	assert.Equal(t, "Important", severity(nil, json.RawMessage(`["severity::HIGH"]`)))
	assert.Equal(t, "Low", severity(nil, json.RawMessage(`["severity::low"]`)))
	assert.Empty(t, severity(nil, json.RawMessage(`["backend","not-severity"]`)))
	assert.Empty(t, severity(nil, json.RawMessage(`["severity::unknown"]`)))
}

func TestMapVulnerabilityUsesSeverityLabelWhenFieldMissing(t *testing.T) {
	issue := validJiraIssue("LTWL-8")
	issue.Fields["labels"] = json.RawMessage(`["java","severity::MEDIUM"]`)

	vulnerability, err := mapVulnerability(issue)
	require.NoError(t, err)
	assert.Equal(t, "Moderate", vulnerability.Severity)
}

func TestStage(t *testing.T) {
	assert.Equal(t, "Submitted", stage(json.RawMessage(`{"name":"New"}`)))
	assert.Equal(t, "Classified", stage(json.RawMessage(`{"name":"Backlog"}`)))
	assert.Equal(t, "Fix in Progress", stage(json.RawMessage(`{"name":"In Progress"}`)))
	assert.Equal(t, "Validation", stage(json.RawMessage(`{"name":"Verified"}`)))
	assert.Equal(t, "Validation", stage(json.RawMessage(`{"name":"Release Pending"}`)))
	assert.Equal(t, "Validation", stage(json.RawMessage(`{"name":"Released"}`)))
	assert.Equal(t, "Validation", stage(json.RawMessage(`{"name":"Closed"}`)))
	assert.Equal(t, "Submitted", stage(json.RawMessage(`{"name":"Unknown"}`)))
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
