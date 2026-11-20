package sync

import (
	"encoding/json"
	"strings"
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
	require.NotNil(t, vulnerability.JiraStatus)
	assert.Equal(t, "New", *vulnerability.JiraStatus)
	assert.Nil(t, vulnerability.JiraResolution)
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

func TestVulnerabilityIDFromSummary(t *testing.T) {
	tests := []struct {
		name    string
		summary string
		want    string
	}{
		{name: "LW ID", summary: "LW-0000-0001 Example component", want: "LW-0000-0001"},
		{name: "CVE ID", summary: "CVE-2025-12345 Example product", want: "CVE-2025-12345"},
		{name: "CVE backport", summary: "[CVE Backport] Example product CVE-2025-12345", want: "CVE-2025-12345"},
		{name: "CVE backport without CVE", summary: "[CVE Backport] Example product"},
		{name: "ID anywhere in summary", summary: "Example product CVE-2025-12345", want: "CVE-2025-12345"},
		{name: "cloned CVE", summary: "CLONE - CVE-2026-50193 jackson-databind: StackOverflowError", want: "CVE-2026-50193"},
		{name: "cloned LW", summary: "CLONE - LW-2026-0066 jackson-databind: DNS lookup", want: "LW-2026-0066"},
		{name: "nested clone", summary: "CLONE - CLONE - CVE-2026-50193 example", want: "CVE-2026-50193"},
		{name: "cloned backport", summary: "CLONE - [CVE Backport] Example product CVE-2025-12345", want: "CVE-2025-12345"},
		{name: "first id wins", summary: "LW-2026-0066 related to CVE-2025-12345", want: "LW-2026-0066"},
		{name: "lowercase normalized", summary: "clone - cve-2026-50193 example", want: "CVE-2026-50193"},
		{name: "no id", summary: "Just a product description"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, vulnerabilityIDFromSummary(tt.summary))
		})
	}
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
	assert.True(t, discardedResolution(json.RawMessage(`{"name":"Duplicate"}`)))
	assert.True(t, discardedResolution(json.RawMessage(`{"name":" duplicate "}`)))
	assert.True(t, discardedResolution(json.RawMessage(`{"name":"Obsolete"}`)))
	assert.True(t, discardedResolution(json.RawMessage(`{"name":"Cannot Reproduce"}`)))
	assert.True(t, discardedResolution(json.RawMessage(`{"name":"Won't Fix"}`)))
	assert.True(t, discardedResolution(json.RawMessage(`{"name":"Won’t Fix"}`)))
	assert.False(t, discardedResolution(json.RawMessage(`{"name":"Not a bug"}`)))
	assert.False(t, discardedResolution(json.RawMessage(`{"name":"Won't do"}`)))
	assert.False(t, discardedResolution(json.RawMessage(`{"name":"Done"}`)))
	assert.False(t, discardedResolution(json.RawMessage(`null`)))
	assert.False(t, discardedResolution(nil))
}

func TestClosedResolutionStatus(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{name: "Won't do", want: "Unremediated"},
		{name: "Won’t Do", want: "Unremediated"},
		{name: "Not a bug", want: "Unremediated"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := json.Marshal(map[string]string{"name": tt.name})
			require.NoError(t, err)
			got, ok := closedResolutionStatus(raw)
			require.True(t, ok)
			assert.Equal(t, tt.want, got)
		})
	}

	for _, name := range []string{"Done", "Duplicate", "Obsolete", "Cannot reproduce", "Won't Fix"} {
		_, ok := closedResolutionStatus(json.RawMessage(`{"name":"` + name + `"}`))
		assert.False(t, ok, name)
	}
}

func TestMapVulnerabilityStoresClosureSelection(t *testing.T) {
	issue := validJiraIssue("LTWL-9")
	issue.Fields["status"] = json.RawMessage(`{"name":"Closed"}`)
	issue.Fields["resolution"] = json.RawMessage(`{"name":"Won't Do"}`)
	issue.Fields[fieldClosureReason] = json.RawMessage(`{"value":"Invalid License"}`)
	issue.Fields["description"] = json.RawMessage(`"Resolution reason: this free text is not the reason"`)

	vulnerability, err := mapVulnerability(issue)
	require.NoError(t, err)
	assert.Equal(t, "Unremediated", vulnerability.Stage)
	require.NotNil(t, vulnerability.ResolutionReason)
	assert.Equal(t, "Invalid License", *vulnerability.ResolutionReason)

	issue.Fields["resolution"] = json.RawMessage(`{"name":"Not a Bug"}`)
	issue.Fields[fieldVEXJustification] = json.RawMessage(`{"value":"Component not Present"}`)
	vulnerability, err = mapVulnerability(issue)
	require.NoError(t, err)
	assert.Equal(t, "Unremediated", vulnerability.Stage)
	require.NotNil(t, vulnerability.ResolutionReason)
	assert.Equal(t, "Component not Present", *vulnerability.ResolutionReason)
}

func TestBeaconDiscardsInternalClosure(t *testing.T) {
	fields := map[string]json.RawMessage{
		"resolution":       json.RawMessage(`{"name":"Won't Do"}`),
		fieldClosureReason: json.RawMessage(`{"value":"Not a Customer"}`),
	}
	assert.True(t, beaconDiscarded(fields))
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
	assert.Equal(t, "On Hold", stage(json.RawMessage(`{"name":"On Hold"}`)))
	assert.Equal(t, "Validation", stage(json.RawMessage(`{"name":"Verified"}`)))
	assert.Equal(t, "Validation", stage(json.RawMessage(`{"name":"Release Pending"}`)))
	assert.Equal(t, "Validation", stage(json.RawMessage(`{"name":"Released"}`)))
	assert.Equal(t, "Validation", stage(json.RawMessage(`{"name":"Closed"}`)))
	assert.Equal(t, "Submitted", stage(json.RawMessage(`{"name":"Unknown"}`)))
}

func TestReleasedOrDone(t *testing.T) {
	fields := func(status, resolution string) map[string]json.RawMessage {
		f := map[string]json.RawMessage{"status": json.RawMessage(`{"name":"` + status + `"}`)}
		if resolution != "" {
			f["resolution"] = json.RawMessage(`{"name":"` + resolution + `"}`)
		}
		return f
	}
	assert.True(t, releasedOrDone(fields("Released", "")))
	assert.True(t, releasedOrDone(fields("released", "")))
	assert.True(t, releasedOrDone(fields("Closed", "Done")))
	assert.True(t, releasedOrDone(fields("Closed", "done")))
	assert.False(t, releasedOrDone(fields("Closed", "Won't Do")))
	assert.False(t, releasedOrDone(fields("Closed", "")))
	assert.False(t, releasedOrDone(fields("Verified", "Done")))
	assert.False(t, releasedOrDone(fields("Release Pending", "")))
	assert.False(t, releasedOrDone(fields("New", "")))
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

// blockerLink builds one issuelinks entry. The linked issue sits on the inward side
// for "is blocked by" links and on whichever side is given for "Incorporates".
func blockerLink(linkName, inward, outward, side, key, issueType, statusCategory string) string {
	linked := `{"key":"` + key + `","fields":{"issuetype":{"name":"` + issueType + `"},"status":{"statusCategory":{"key":"` + statusCategory + `"}}}}`
	return `{"type":{"name":"` + linkName + `","inward":"` + inward + `","outward":"` + outward + `"},"` + side + `":` + linked + `}`
}

func ivtWithLinks(links ...string) jira_client.JiraIssue {
	return jira_client.JiraIssue{Key: "LTWL-1", Fields: map[string]json.RawMessage{
		"issuelinks": json.RawMessage("[" + strings.Join(links, ",") + "]"),
	}}
}

func TestOnHoldReasons(t *testing.T) {
	buildTask := func(key, category string) string {
		return blockerLink("Blocks", "is blocked by", "blocks", "inwardIssue", key, "Build Task", category)
	}
	workTicket := func(key, issueType, category string) string {
		return blockerLink("Blocks", "is blocked by", "blocks", "inwardIssue", key, issueType, category)
	}
	incorporates := func(side, key, category string) string {
		return blockerLink("Incorporates", "is incorporated by", "incorporates", side, key, "Component Upgrade", category)
	}

	tests := []struct {
		name   string
		links  []string
		labels map[string][]string
		want   []string
	}{
		{
			name:   "open build task label becomes a slug",
			links:  []string{buildTask("VBT-1", "indeterminate")},
			labels: map[string][]string{"VBT-1": {"unrelated", "blocked:missing-sources"}},
			want:   []string{"missing-sources"},
		},
		{
			name:   "label prefix and slug are lowercased",
			links:  []string{buildTask("VBT-1", "new")},
			labels: map[string][]string{"VBT-1": {"Blocked:Missing-Sources"}},
			want:   []string{"missing-sources"},
		},
		{
			name:   "done build task is not blocking",
			links:  []string{buildTask("VBT-1", "done")},
			labels: map[string][]string{"VBT-1": {"blocked:missing-sources"}},
			want:   nil,
		},
		{
			name:   "open build task without a blocked label adds nothing",
			links:  []string{buildTask("VBT-1", "new")},
			labels: map[string][]string{"VBT-1": {"java"}},
			want:   nil,
		},
		{
			name:   "empty slug after the prefix is ignored",
			links:  []string{buildTask("VBT-1", "new")},
			labels: map[string][]string{"VBT-1": {"blocked:"}},
			want:   nil,
		},
		{
			name:   "build task with no fetched labels adds nothing",
			links:  []string{buildTask("VBT-1", "new")},
			labels: map[string][]string{},
			want:   nil,
		},
		{
			name:   "component upgrade on the outward side",
			links:  []string{incorporates("outwardIssue", "CT-1", "new")},
			labels: map[string][]string{"CT-1": {"blocked:monorepo"}},
			want:   []string{"monorepo"},
		},
		{
			name:   "component upgrade on the inward side",
			links:  []string{incorporates("inwardIssue", "CT-1", "new")},
			labels: map[string][]string{"CT-1": {"blocked:monorepo"}},
			want:   []string{"monorepo"},
		},
		{
			name:   "done component upgrade is not blocking",
			links:  []string{incorporates("outwardIssue", "CT-1", "done")},
			labels: map[string][]string{"CT-1": {"blocked:monorepo"}},
			want:   nil,
		},
		{
			name:  "open work ticket yields blocked-by-ticket",
			links: []string{workTicket("LWLP-1", "Bug", "indeterminate")},
			want:  []string{"blocked-by-ticket"},
		},
		{
			name:  "done work ticket is not blocking",
			links: []string{workTicket("LWLP-1", "Bug", "done")},
			want:  nil,
		},
		{
			name: "outward blocks link is not a blocker of this issue",
			links: []string{
				blockerLink("Blocks", "is blocked by", "blocks", "outwardIssue", "LWLP-2", "Bug", "new"),
			},
			want: nil,
		},
		{
			name: "other link types are ignored",
			links: []string{
				blockerLink("Relates", "relates to", "relates to", "inwardIssue", "LWLP-3", "Bug", "new"),
			},
			want: nil,
		},
		{
			name: "several blockers are merged, de-duplicated and sorted",
			links: []string{
				workTicket("LWLP-1", "Story", "new"),
				workTicket("LWLP-2", "Task", "new"),
				buildTask("VBT-1", "new"),
				buildTask("VBT-2", "new"),
				incorporates("outwardIssue", "CT-1", "new"),
			},
			labels: map[string][]string{
				"VBT-1": {"blocked:synthesis-conflict", "blocked:ant-build"},
				"VBT-2": {"blocked:ant-build"},
				"CT-1":  {"blocked:integration-failure"},
			},
			want: []string{"ant-build", "blocked-by-ticket", "integration-failure", "synthesis-conflict"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blockers := make(map[string]blockerDetails, len(tt.labels))
			for key, labels := range tt.labels {
				blockers[key] = blockerDetails{Labels: labels}
			}
			assert.Equal(t, tt.want, onHoldReasons(ivtWithLinks(tt.links...), blockers))
		})
	}
}

// blockerIssue is a Build Task or Component Upgrade as returned when its blocker
// fields (labels, issuelinks, issuetype) are requested.
func blockerIssue(key, issueType string, labels []string, links ...string) jira_client.JiraIssue {
	rawLabels, _ := json.Marshal(labels)
	return jira_client.JiraIssue{Key: key, Fields: map[string]json.RawMessage{
		"labels":     rawLabels,
		"issuetype":  json.RawMessage(`{"name":"` + issueType + `"}`),
		"issuelinks": json.RawMessage("[" + strings.Join(links, ",") + "]"),
	}}
}

func TestBlockerDetailsOf(t *testing.T) {
	blockedBy := func(key, category string) string {
		return blockerLink("Blocks", "is blocked by", "blocks", "inwardIssue", key, "Bug", category)
	}
	const ct, buildTask = "Component Upgrade", "Build Task"

	noLinkField := blockerIssue("CT-1", ct, []string{"blocked:monorepo"})
	delete(noLinkField.Fields, "issuelinks")
	noIssueType := blockerIssue("CT-1", ct, []string{}, blockedBy("LWLP-1", "new"))
	delete(noIssueType.Fields, "issuetype")
	malformedLinks := blockerIssue("CT-1", ct, []string{})
	malformedLinks.Fields["issuelinks"] = json.RawMessage(`{"broken"`)

	tests := []struct {
		name  string
		issue jira_client.JiraIssue
		want  bool
	}{
		{name: "component upgrade blocked by an open ticket", issue: blockerIssue("CT-1", ct, []string{}, blockedBy("LWLP-1", "indeterminate")), want: true},
		{name: "component upgrade blocked by a ticket not yet started", issue: blockerIssue("CT-1", ct, []string{}, blockedBy("LWLP-1", "new")), want: true},
		{name: "component upgrade blocked by a done ticket", issue: blockerIssue("CT-1", ct, []string{}, blockedBy("LWLP-1", "done")), want: false},
		{
			name: "an open ticket that only appears after a done one still blocks",
			issue: blockerIssue("CT-1", ct, []string{},
				blockedBy("LWLP-1", "done"), blockedBy("LWLP-2", "new")),
			want: true,
		},
		{
			name: "component upgrade that blocks another ticket is not blocked",
			issue: blockerIssue("CT-1", ct, []string{},
				blockerLink("Blocks", "is blocked by", "blocks", "outwardIssue", "LWLP-1", "Bug", "new")),
			want: false,
		},
		{
			name: "other link types are ignored",
			issue: blockerIssue("CT-1", ct, []string{},
				blockerLink("Relates", "relates to", "relates to", "inwardIssue", "LWLP-1", "Bug", "new")),
			want: false,
		},
		{name: "component upgrade with no links", issue: blockerIssue("CT-1", ct, []string{}), want: false},
		{name: "component upgrade with no links field", issue: noLinkField, want: false},
		{name: "component upgrade with malformed links", issue: malformedLinks, want: false},
		{name: "build task links are not read", issue: blockerIssue("VBT-1", buildTask, []string{}, blockedBy("LWLP-1", "new")), want: false},
		{name: "unknown issue type is not read", issue: noIssueType, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := blockerDetailsOf(tt.issue)
			assert.Equal(t, tt.want, got.BlockedByTicket)
			assert.Equal(t, labelsOf(tt.issue), got.Labels, "labels are always carried over")
		})
	}
}

func TestOnHoldReasonsFromComponentUpgradeBlockers(t *testing.T) {
	incorporates := func(key, category string) string {
		return blockerLink("Incorporates", "is incorporated by", "incorporates", "outwardIssue", key, "Component Upgrade", category)
	}
	workTicket := blockerLink("Blocks", "is blocked by", "blocks", "inwardIssue", "LWLP-1", "Bug", "new")

	tests := []struct {
		name     string
		links    []string
		blockers map[string]blockerDetails
		want     []string
	}{
		{
			name:     "component upgrade blocked by a ticket and no label",
			links:    []string{incorporates("CT-1", "indeterminate")},
			blockers: map[string]blockerDetails{"CT-1": {BlockedByTicket: true}},
			want:     []string{"blocked-by-ticket"},
		},
		{
			name:     "label and blocking ticket on the same component upgrade",
			links:    []string{incorporates("CT-1", "indeterminate")},
			blockers: map[string]blockerDetails{"CT-1": {Labels: []string{"blocked:monorepo"}, BlockedByTicket: true}},
			want:     []string{"blocked-by-ticket", "monorepo"},
		},
		{
			name:     "same reason from the IVT's own ticket is stored once",
			links:    []string{workTicket, incorporates("CT-1", "indeterminate")},
			blockers: map[string]blockerDetails{"CT-1": {BlockedByTicket: true}},
			want:     []string{"blocked-by-ticket"},
		},
		{
			name:     "done component upgrade is ignored even if its details say blocked",
			links:    []string{incorporates("CT-1", "done")},
			blockers: map[string]blockerDetails{"CT-1": {BlockedByTicket: true}},
			want:     nil,
		},
		{
			name:     "details of a ticket the IVT is not linked to are ignored",
			links:    []string{incorporates("CT-1", "indeterminate")},
			blockers: map[string]blockerDetails{"CT-2": {BlockedByTicket: true}},
			want:     nil,
		},
		{
			name:     "component upgrade neither labelled nor blocked adds nothing",
			links:    []string{incorporates("CT-1", "indeterminate")},
			blockers: map[string]blockerDetails{"CT-1": {Labels: []string{"java"}}},
			want:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, onHoldReasons(ivtWithLinks(tt.links...), tt.blockers))
		})
	}
}

// linkWithStatus builds one issuelinks entry like blockerLink, but the linked issue also
// carries its status name, which Jira embeds next to the status category.
func linkWithStatus(linkName, inward, outward, side, key, issueType, statusName, statusCategory string) string {
	linked := `{"key":"` + key + `","fields":{"issuetype":{"name":"` + issueType + `"},"status":{"name":"` + statusName + `","statusCategory":{"key":"` + statusCategory + `"}}}}`
	return `{"type":{"name":"` + linkName + `","inward":"` + inward + `","outward":"` + outward + `"},"` + side + `":` + linked + `}`
}

// incorporatedCT builds an "Incorporates" link to a Component Upgrade with the given status.
func incorporatedCT(side, key, statusName, statusCategory string) string {
	return linkWithStatus("Incorporates", "is incorporated by", "incorporates", side, key, "Component Upgrade", statusName, statusCategory)
}

func TestOnHoldComponentUpgrades(t *testing.T) {
	tests := []struct {
		name  string
		links []string
		want  []string
	}{
		{
			name:  "on hold component upgrade on the outward side",
			links: []string{incorporatedCT("outwardIssue", "CT-1", "On Hold", "indeterminate")},
			want:  []string{"CT-1"},
		},
		{
			name:  "on hold component upgrade on the inward side",
			links: []string{incorporatedCT("inwardIssue", "CT-1", "On Hold", "indeterminate")},
			want:  []string{"CT-1"},
		},
		{
			name:  "status name ignores case and surrounding spaces",
			links: []string{incorporatedCT("outwardIssue", "CT-1", " on hold ", "indeterminate")},
			want:  []string{"CT-1"},
		},
		{
			name:  "component upgrade in progress is not on hold",
			links: []string{incorporatedCT("outwardIssue", "CT-1", "In Progress", "indeterminate")},
			want:  nil,
		},
		{
			name:  "on hold status in a done category is not open",
			links: []string{incorporatedCT("outwardIssue", "CT-1", "On Hold", "done")},
			want:  nil,
		},
		{
			name: "on hold ticket that is not a component upgrade",
			links: []string{linkWithStatus("Incorporates", "is incorporated by", "incorporates",
				"outwardIssue", "LWLP-1", "Story", "On Hold", "indeterminate")},
			want: nil,
		},
		{
			name: "on hold component upgrade linked another way",
			links: []string{linkWithStatus("Relates", "relates to", "relates to",
				"inwardIssue", "CT-1", "Component Upgrade", "On Hold", "indeterminate")},
			want: nil,
		},
		{
			name:  "link without a status name",
			links: []string{blockerLink("Incorporates", "is incorporated by", "incorporates", "outwardIssue", "CT-1", "Component Upgrade", "indeterminate")},
			want:  nil,
		},
		{
			name: "several on hold component upgrades keep link order",
			links: []string{
				incorporatedCT("outwardIssue", "CT-2", "On Hold", "indeterminate"),
				incorporatedCT("inwardIssue", "CT-1", "On Hold", "indeterminate"),
				incorporatedCT("outwardIssue", "CT-3", "In Progress", "indeterminate"),
			},
			want: []string{"CT-2", "CT-1"},
		},
		{
			name: "the same component upgrade linked twice is listed once",
			links: []string{
				incorporatedCT("outwardIssue", "CT-1", "On Hold", "indeterminate"),
				incorporatedCT("outwardIssue", "CT-1", "On Hold", "indeterminate"),
			},
			want: []string{"CT-1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, onHoldComponentUpgrades(ivtWithLinks(tt.links...)))
		})
	}
}

func TestOnHoldComponentUpgradesHandlesMissingOrMalformedLinks(t *testing.T) {
	assert.Nil(t, onHoldComponentUpgrades(jira_client.JiraIssue{Key: "LTWL-1", Fields: map[string]json.RawMessage{}}))
	assert.Nil(t, onHoldComponentUpgrades(jira_client.JiraIssue{Key: "LTWL-1", Fields: map[string]json.RawMessage{
		"issuelinks": json.RawMessage(`{"broken"`),
	}}))
}

func TestBlockerReasonsUsesOnlyTheGivenBlockers(t *testing.T) {
	blockers := map[string]blockerDetails{
		"CT-1": {Labels: []string{"blocked:monorepo"}},
		"CT-2": {Labels: []string{"blocked:ant-build"}, BlockedByTicket: true},
	}

	assert.Equal(t, []string{"monorepo"}, blockerReasons([]string{"CT-1"}, false, blockers))
	assert.Equal(t, []string{"ant-build", "blocked-by-ticket", "monorepo"}, blockerReasons([]string{"CT-1", "CT-2"}, false, blockers))
	assert.Equal(t, []string{"blocked-by-ticket", "monorepo"}, blockerReasons([]string{"CT-1"}, true, blockers))
	assert.Nil(t, blockerReasons(nil, false, blockers))
}

func TestOnHoldReasonsHandlesMissingOrMalformedLinks(t *testing.T) {
	assert.Nil(t, onHoldReasons(jira_client.JiraIssue{Key: "LTWL-1", Fields: map[string]json.RawMessage{}}, nil))
	assert.Nil(t, onHoldReasons(jira_client.JiraIssue{Key: "LTWL-1", Fields: map[string]json.RawMessage{
		"issuelinks": json.RawMessage(`{"broken"`),
	}}, nil))
}

func TestOnHoldBlockersReturnsKeysToFetchLabelsFor(t *testing.T) {
	ivt := ivtWithLinks(
		blockerLink("Blocks", "is blocked by", "blocks", "inwardIssue", "VBT-1", "Build Task", "new"),
		blockerLink("Blocks", "is blocked by", "blocks", "inwardIssue", "VBT-2", "Build Task", "done"),
		blockerLink("Incorporates", "is incorporated by", "incorporates", "outwardIssue", "CT-1", "Component Upgrade", "new"),
		blockerLink("Blocks", "is blocked by", "blocks", "inwardIssue", "LWLP-1", "Bug", "new"),
	)
	keys, blockedByTicket := onHoldBlockers(ivt)
	assert.Equal(t, []string{"VBT-1", "CT-1"}, keys)
	assert.True(t, blockedByTicket)
}

func TestCustomerIDsFromLabels(t *testing.T) {
	cases := []struct {
		name   string
		labels string
		want   []string
	}{
		{name: "single", labels: `["CID-12345"]`, want: []string{"CID-12345"}},
		{name: "case insensitive prefix", labels: `["cid-42"]`, want: []string{"CID-42"}},
		{name: "multiple distinct", labels: `["CID-1","other","CID-2"]`, want: []string{"CID-1", "CID-2"}},
		{name: "dedupes", labels: `["CID-7","cid-7"]`, want: []string{"CID-7"}},
		{name: "ignores non-digit suffix", labels: `["CID-abc","CID-12x"]`, want: []string{}},
		{name: "ignores unprefixed and partial", labels: `["12345","severity::low","CID-"]`, want: []string{}},
		{name: "trims whitespace", labels: `[" CID-9 "]`, want: []string{"CID-9"}},
		{name: "no labels field", labels: ``, want: nil},
		{name: "empty list", labels: `[]`, want: []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, customerIDsFromLabels(json.RawMessage(tc.labels)))
		})
	}
}

func validJiraIssue(key string) jira_client.JiraIssue {
	return jira_client.JiraIssue{Key: key, Fields: map[string]json.RawMessage{
		"summary": json.RawMessage(`"LW-0000-0001 Example component"`),
		"created": json.RawMessage(`"2026-08-15T23:30:00-0200"`),
		"updated": json.RawMessage(`"2026-08-16T23:40:00-0200"`),
		"status":  json.RawMessage(`{"name":"New"}`),
	}}
}
