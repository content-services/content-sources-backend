package sync

import (
	"encoding/json"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/clients/jira_client"
	"github.com/stretchr/testify/assert"
)

func ivtWith(key, status, labelsJSON string) jira_client.JiraIssue {
	return jira_client.JiraIssue{Key: key, Fields: map[string]json.RawMessage{
		"status": json.RawMessage(`{"name":"` + status + `"}`),
		"labels": json.RawMessage(labelsJSON),
	}}
}

func ctIncorporating(key string, memberKeys ...string) jira_client.JiraIssue {
	links := make([]string, 0, len(memberKeys))
	for _, m := range memberKeys {
		// Real Jira shape: the "Incorporates" link type's matchable verb is the
		// lowercase outward string "incorporates"; the member IVT is the outwardIssue.
		links = append(links, `{"type":{"name":"Incorporates","inward":"is incorporated by","outward":"incorporates"},"outwardIssue":{"key":"`+m+`"}}`)
	}
	raw := "[" + join(links, ",") + "]"
	return jira_client.JiraIssue{Key: key, Fields: map[string]json.RawMessage{
		"issuelinks": json.RawMessage(raw),
	}}
}

func join(parts []string, sep string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += sep
		}
		out += p
	}
	return out
}

func TestIsShipReady(t *testing.T) {
	for _, s := range []string{"Verified", "Release Pending", "Released", "Closed", "verified"} {
		assert.True(t, isShipReady(s), s)
	}
	for _, s := range []string{"New", "To Do", "In Progress", "Review", "On Hold", ""} {
		assert.False(t, isShipReady(s), s)
	}
}

func TestBuildInheritedCIDsUnionsShipReadyMembers(t *testing.T) {
	labelled := ivtWith("LTWL-1", "Released", `["CID-663171"]`)
	unlabelled := ivtWith("LTWL-2", "Released", `[]`)
	ct := ctIncorporating("CT-1", "LTWL-1", "LTWL-2")
	got := buildInheritedCIDs([]jira_client.JiraIssue{labelled, unlabelled}, []jira_client.JiraIssue{ct})
	assert.Equal(t, []string{"663171"}, got["LTWL-1"])
	assert.Equal(t, []string{"663171"}, got["LTWL-2"])
}

func TestBuildInheritedCIDsIgnoresNonShipReadySource(t *testing.T) {
	newLabelled := ivtWith("LTWL-1", "New", `["CID-663171"]`)
	doneUnlabelled := ivtWith("LTWL-2", "Released", `[]`)
	ct := ctIncorporating("CT-1", "LTWL-1", "LTWL-2")
	got := buildInheritedCIDs([]jira_client.JiraIssue{newLabelled, doneUnlabelled}, []jira_client.JiraIssue{ct})
	assert.Empty(t, got["LTWL-2"])
}

func TestBuildInheritedCIDsSkipsUnknownAndMalformed(t *testing.T) {
	labelled := ivtWith("LTWL-1", "Released", `["CID-663171"]`)
	ctUnknownMember := ctIncorporating("CT-1", "LTWL-1", "LTWL-404") // LTWL-404 not fetched
	ctBadLinks := jira_client.JiraIssue{Key: "CT-2", Fields: map[string]json.RawMessage{
		"issuelinks": json.RawMessage(`not-json`)}}
	got := buildInheritedCIDs([]jira_client.JiraIssue{labelled},
		[]jira_client.JiraIssue{ctUnknownMember, ctBadLinks})
	assert.NotContains(t, got, "LTWL-404")
	assert.Equal(t, []string{"663171"}, got["LTWL-1"])
}
