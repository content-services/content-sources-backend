package sync

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/clients/jira_client"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeJira struct {
	fields      []jira_client.JiraField
	fieldErr    error
	pages       map[string]jira_client.JiraPage
	searchErr   error
	issues      map[string]jira_client.JiraIssue
	issueCalls  []string
	issueFields [][]string
	searchCalls []string
}

func (f *fakeJira) Fields(context.Context) ([]jira_client.JiraField, error) {
	return f.fields, f.fieldErr
}

func (f *fakeJira) Search(_ context.Context, _ string, _ []string, token string) (jira_client.JiraPage, error) {
	f.searchCalls = append(f.searchCalls, token)
	if f.searchErr != nil {
		return jira_client.JiraPage{}, f.searchErr
	}
	return f.pages[token], nil
}

func (f *fakeJira) Issue(_ context.Context, key string, fields []string) (jira_client.JiraIssue, error) {
	f.issueCalls = append(f.issueCalls, key)
	f.issueFields = append(f.issueFields, fields)
	issue, exists := f.issues[key]
	if !exists {
		return jira_client.JiraIssue{}, errors.New("missing fake issue")
	}
	return issue, nil
}

type fakeVulnerabilityStore struct {
	saved   []dao.LightwellVulnerabilityInput
	outcome dao.LightwellVulnerabilitySaveOutcome
}

func (f *fakeVulnerabilityStore) Save(_ context.Context, input dao.LightwellVulnerabilityInput) (dao.LightwellVulnerabilitySaveOutcome, error) {
	f.saved = append(f.saved, input)
	return f.outcome, nil
}

func TestIngestorSyncPaginatesAndLoadsNewRelationships(t *testing.T) {
	vulnerability := validJiraIssue("LTWL-1")
	vulnerability.Fields["issuelinks"] = json.RawMessage(`[{
		"type":{"outward":"relates to"},"outwardIssue":{"key":"BATCH-1"}
	}]`)
	batch := jira_client.JiraIssue{Key: "BATCH-1", Fields: map[string]json.RawMessage{
		"issuelinks": json.RawMessage(`[{
			"type":{"outward":"is child of"},"outwardIssue":{"key":"EPIC-1"}
		}]`),
	}}
	epic := jira_client.JiraIssue{Key: "EPIC-1", Fields: map[string]json.RawMessage{
		"customfield_account": json.RawMessage(`["123",{"value":"456"},"123"]`),
		"issuelinks":          json.RawMessage(`[{"type":{"inward":"is triggered by"},"inwardIssue":{"key":"TICKET-1"}}]`),
	}}

	jira := &fakeJira{
		fields: []jira_client.JiraField{{ID: "customfield_account", Name: "Account Number"}},
		pages: map[string]jira_client.JiraPage{
			"":     {NextPageToken: "next"},
			"next": {Issues: []jira_client.JiraIssue{vulnerability}},
		},
		issues: map[string]jira_client.JiraIssue{"BATCH-1": batch, "EPIC-1": epic},
	}
	store := &fakeVulnerabilityStore{outcome: dao.LightwellVulnerabilityInserted}

	summary, err := NewIngestor(jira, store, nil).Sync(context.Background())
	require.NoError(t, err)
	assert.Equal(t, SyncSummary{Inserted: 1}, summary)
	require.Len(t, store.saved, 1)
	assert.Equal(t, "LTWL-1", store.saved[0].VulnerabilityKey)
	assert.Equal(t, "LW-0000-0001", store.saved[0].VulnerabilityID)
	assert.Equal(t, []dao.LightwellVulnerabilityTicket{{TicketID: "TICKET-1", CustomerID: "123"}}, store.saved[0].Tickets)
	assert.Equal(t, []string{"", "next"}, jira.searchCalls)
	assert.Equal(t, []string{"BATCH-1", "EPIC-1"}, jira.issueCalls)
}

func TestIngestorReloadsRelationshipsForExistingIssue(t *testing.T) {
	issue := validJiraIssue("LTWL-1")
	issue.Fields["issuelinks"] = json.RawMessage(`[{"type":{"outward":"relates to"},"outwardIssue":{"key":"BATCH-1"}}]`)
	batch := jira_client.JiraIssue{Key: "BATCH-1", Fields: map[string]json.RawMessage{
		"parent": json.RawMessage(`{"key":"EPIC-1"}`),
	}}
	epic := jira_client.JiraIssue{Key: "EPIC-1", Fields: map[string]json.RawMessage{
		"customfield_account": json.RawMessage(`["123"]`),
		"issuelinks":          json.RawMessage(`[{"type":{"inward":"is triggered by"},"inwardIssue":{"key":"TICKET-1"}}]`),
	}}
	jira := &fakeJira{
		fields: []jira_client.JiraField{{ID: "customfield_account", Name: "Account Number"}},
		pages:  map[string]jira_client.JiraPage{"": {Issues: []jira_client.JiraIssue{issue}}},
		issues: map[string]jira_client.JiraIssue{"BATCH-1": batch, "EPIC-1": epic},
	}
	store := &fakeVulnerabilityStore{outcome: dao.LightwellVulnerabilityUnchanged}

	summary, err := NewIngestor(jira, store, nil).Sync(context.Background())
	require.NoError(t, err)
	assert.Equal(t, SyncSummary{Unchanged: 1}, summary)
	assert.Equal(t, []string{"BATCH-1", "EPIC-1"}, jira.issueCalls)
	require.Len(t, store.saved, 1)
	assert.Equal(t, []dao.LightwellVulnerabilityTicket{{TicketID: "TICKET-1", CustomerID: "123"}}, store.saved[0].Tickets)
}

func TestIngestorLoadsMultipleBatchRelationships(t *testing.T) {
	vulnerability := validJiraIssue("LTWL-1")
	vulnerability.Fields["issuelinks"] = json.RawMessage(`[
		{"type":{"outward":"relates to"},"outwardIssue":{"key":"BATCH-1"}},
		{"type":{"outward":"relates to"},"outwardIssue":{"key":"BATCH-2"}}
	]`)
	batch1 := jira_client.JiraIssue{Key: "BATCH-1", Fields: map[string]json.RawMessage{
		"parent": json.RawMessage(`{"key":"EPIC-1"}`),
	}}
	batch2 := jira_client.JiraIssue{Key: "BATCH-2", Fields: map[string]json.RawMessage{
		"parent": json.RawMessage(`{"key":"EPIC-2"}`),
	}}
	epic1 := jira_client.JiraIssue{Key: "EPIC-1", Fields: map[string]json.RawMessage{
		"customfield_account": json.RawMessage(`["111"]`),
		"issuelinks":          json.RawMessage(`[{"type":{"inward":"is triggered by"},"inwardIssue":{"key":"TICKET-1"}}]`),
	}}
	epic2 := jira_client.JiraIssue{Key: "EPIC-2", Fields: map[string]json.RawMessage{
		"customfield_account": json.RawMessage(`["222"]`),
		"issuelinks":          json.RawMessage(`[{"type":{"inward":"is triggered by"},"inwardIssue":{"key":"TICKET-2"}}]`),
	}}
	jira := &fakeJira{
		fields: []jira_client.JiraField{{ID: "customfield_account", Name: "Account Number"}},
		pages:  map[string]jira_client.JiraPage{"": {Issues: []jira_client.JiraIssue{vulnerability}}},
		issues: map[string]jira_client.JiraIssue{"BATCH-1": batch1, "BATCH-2": batch2, "EPIC-1": epic1, "EPIC-2": epic2},
	}
	store := &fakeVulnerabilityStore{outcome: dao.LightwellVulnerabilityInserted}

	_, err := NewIngestor(jira, store, nil).Sync(context.Background())
	require.NoError(t, err)
	require.Len(t, store.saved, 1)
	assert.Equal(t, []dao.LightwellVulnerabilityTicket{
		{TicketID: "TICKET-1", CustomerID: "111"},
		{TicketID: "TICKET-2", CustomerID: "222"},
	}, store.saved[0].Tickets)
}

func TestIngestorStoresEpicFromBatchParent(t *testing.T) {
	vulnerability := validJiraIssue("LTWL-1")
	vulnerability.Fields["issuelinks"] = json.RawMessage(`[{ 
		"type":{"outward":"relates to"},"outwardIssue":{"key":"BATCH-1"}
	}]`)
	batch := jira_client.JiraIssue{Key: "BATCH-1", Fields: map[string]json.RawMessage{
		"parent": json.RawMessage(`{"key":"EPIC-1"}`),
	}}
	epic := jira_client.JiraIssue{Key: "EPIC-1", Fields: map[string]json.RawMessage{
		"customfield_account": json.RawMessage(`["789"]`),
		"issuelinks":          json.RawMessage(`[{"type":{"inward":"is triggered by"},"inwardIssue":{"key":"TICKET-1"}}]`),
	}}
	jira := &fakeJira{
		fields: []jira_client.JiraField{{ID: "customfield_account", Name: "Account Number"}},
		pages:  map[string]jira_client.JiraPage{"": {Issues: []jira_client.JiraIssue{vulnerability}}},
		issues: map[string]jira_client.JiraIssue{"BATCH-1": batch, "EPIC-1": epic},
	}
	store := &fakeVulnerabilityStore{outcome: dao.LightwellVulnerabilityInserted}

	_, err := NewIngestor(jira, store, nil).Sync(context.Background())
	require.NoError(t, err)
	require.Len(t, store.saved, 1)
	assert.Equal(t, []string{"parent", "issuelinks"}, jira.issueFields[0])
	assert.Equal(t, []dao.LightwellVulnerabilityTicket{{TicketID: "TICKET-1", CustomerID: "789"}}, store.saved[0].Tickets)
}

func TestIngestorLinksCustomerWithoutSupportTicket(t *testing.T) {
	vulnerability := validJiraIssue("LTWL-1")
	vulnerability.Fields["issuelinks"] = json.RawMessage(`[{
		"type":{"outward":"relates to"},"outwardIssue":{"key":"BATCH-1"}
	}]`)
	batch := jira_client.JiraIssue{Key: "BATCH-1", Fields: map[string]json.RawMessage{
		"parent": json.RawMessage(`{"key":"EPIC-1"}`),
	}}
	epic := jira_client.JiraIssue{Key: "EPIC-1", Fields: map[string]json.RawMessage{
		"customfield_account": json.RawMessage(`["789"]`),
	}}
	jira := &fakeJira{
		fields: []jira_client.JiraField{{ID: "customfield_account", Name: "Account Number"}},
		pages:  map[string]jira_client.JiraPage{"": {Issues: []jira_client.JiraIssue{vulnerability}}},
		issues: map[string]jira_client.JiraIssue{"BATCH-1": batch, "EPIC-1": epic},
	}
	store := &fakeVulnerabilityStore{outcome: dao.LightwellVulnerabilityInserted}

	_, err := NewIngestor(jira, store, nil).Sync(context.Background())
	require.NoError(t, err)
	require.Len(t, store.saved, 1)
	assert.Equal(t, []dao.LightwellVulnerabilityTicket{{TicketID: "", CustomerID: "789"}}, store.saved[0].Tickets)
}

func TestIngestorContinuesAfterIssueMappingFailure(t *testing.T) {
	bad := validJiraIssue("LTWL-BAD")
	delete(bad.Fields, "created")
	good := validJiraIssue("LTWL-GOOD")
	jira := &fakeJira{pages: map[string]jira_client.JiraPage{"": {Issues: []jira_client.JiraIssue{bad, good}}}}
	store := &fakeVulnerabilityStore{outcome: dao.LightwellVulnerabilityInserted}

	summary, err := NewIngestor(jira, store, nil).Sync(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, summary.Inserted)
	assert.Equal(t, 1, summary.Failed)
	assert.Len(t, summary.Failures, 1)
	assert.Len(t, store.saved, 1)
}

func TestIngestorSearchFailureWritesNothing(t *testing.T) {
	jira := &fakeJira{searchErr: errors.New("search failed")}
	store := &fakeVulnerabilityStore{outcome: dao.LightwellVulnerabilityInserted}

	_, err := NewIngestor(jira, store, nil).Sync(context.Background())
	assert.ErrorContains(t, err, "search failed")
	assert.Empty(t, store.saved)
}

func TestIngestorRejectsAmbiguousAccountField(t *testing.T) {
	jira := &fakeJira{fields: []jira_client.JiraField{
		{ID: "customfield_1", Name: "Account Number"},
		{ID: "customfield_2", Name: "account number"},
	}}

	_, err := NewIngestor(jira, &fakeVulnerabilityStore{}, nil).Sync(context.Background())
	assert.ErrorContains(t, err, "multiple Jira fields")
}

type fakeAdvisories struct {
	rows []dao.LightwellAdvisoryInput
	err  error
}

func (f fakeAdvisories) List(_ context.Context, offset int, limit int) ([]dao.LightwellAdvisoryInput, int64, error) {
	if f.err != nil {
		return nil, 0, f.err
	}
	total := int64(len(f.rows))
	if offset >= len(f.rows) {
		return nil, total, nil
	}
	end := offset + limit
	if end > len(f.rows) {
		end = len(f.rows)
	}
	return f.rows[offset:end], total, nil
}

func TestIngestorPromotesClosedIssueWhenPublished(t *testing.T) {
	issue := validJiraIssue("LTWL-1")
	issue.Fields["status"] = json.RawMessage(`{"name":"Closed"}`)
	issue.Fields["description"] = json.RawMessage(`{
		"type":"doc","content":[
			{"type":"paragraph","content":[{"type":"text","text":"Component: com.example:demo-lib"}]},
			{"type":"paragraph","content":[{"type":"text","text":"version: 1.2.3"}]}
		]}`)
	issue.Fields["labels"] = json.RawMessage(`["java"]`)
	jira := &fakeJira{pages: map[string]jira_client.JiraPage{"": {Issues: []jira_client.JiraIssue{issue}}}}
	store := &fakeVulnerabilityStore{outcome: dao.LightwellVulnerabilityInserted}
	advisories := fakeAdvisories{rows: []dao.LightwellAdvisoryInput{{
		RepoName:      "lightwell/java/remediated",
		AdvisoryID:    "x_DEMO-LW-0000-0001-1.2.3",
		PackageName:   "com.example:demo-lib",
		FixedVersions: []string{"1.2.3.build-00001"},
	}}}

	_, err := NewIngestor(jira, store, advisories).Sync(context.Background())
	require.NoError(t, err)
	require.Len(t, store.saved, 1)
	assert.Equal(t, "Lightwell Network", store.saved[0].Stage)
}

func TestIngestorKeepsValidationWhenNotPublished(t *testing.T) {
	issue := validJiraIssue("LTWL-1")
	issue.Fields["status"] = json.RawMessage(`{"name":"Closed"}`)
	jira := &fakeJira{pages: map[string]jira_client.JiraPage{"": {Issues: []jira_client.JiraIssue{issue}}}}
	store := &fakeVulnerabilityStore{outcome: dao.LightwellVulnerabilityInserted}

	_, err := NewIngestor(jira, store, fakeAdvisories{}).Sync(context.Background())
	require.NoError(t, err)
	require.Len(t, store.saved, 1)
	assert.Equal(t, "Validation", store.saved[0].Stage)
}

func TestIngestorPromotesEveryValidationStatusWhenPublished(t *testing.T) {
	advisories := fakeAdvisories{rows: []dao.LightwellAdvisoryInput{matchingJavaAdvisory("LW-0000-0001")}}
	for _, status := range []string{"Closed", "Verified", "Release Pending", "Released"} {
		t.Run(status, func(t *testing.T) {
			issue := publishedJiraIssue("LTWL-1", "LW-0000-0001 Example", status, "java", "com.example:demo-lib", "1.2.3")
			jira := &fakeJira{pages: map[string]jira_client.JiraPage{"": {Issues: []jira_client.JiraIssue{issue}}}}
			store := &fakeVulnerabilityStore{outcome: dao.LightwellVulnerabilityInserted}

			_, err := NewIngestor(jira, store, advisories).Sync(context.Background())
			require.NoError(t, err)
			require.Len(t, store.saved, 1)
			assert.Equal(t, "Lightwell Network", store.saved[0].Stage)
		})
	}
}

func TestIngestorDoesNotMissMatchingClosedIssues(t *testing.T) {
	issues := []jira_client.JiraIssue{
		publishedJiraIssue("LTWL-1", "LW-0000-0001 Example", "Closed", "java", "com.example:demo-lib", "1.2.3"),
		publishedJiraIssue("LTWL-2", "LW-0000-0002 Example", "Closed", "python", "demo-pkg", "4.0.0"),
		publishedJiraIssue("LTWL-3", "LW-0000-0003 Example", "Closed", "java", "com.example:other-lib", "9.9.9"),
		publishedJiraIssue("LTWL-4", "LW-0000-0001 Example", "In Progress", "java", "com.example:demo-lib", "1.2.3"),
	}
	jira := &fakeJira{pages: map[string]jira_client.JiraPage{"": {Issues: issues}}}
	store := &fakeVulnerabilityStore{outcome: dao.LightwellVulnerabilityInserted}
	advisories := fakeAdvisories{rows: []dao.LightwellAdvisoryInput{
		matchingJavaAdvisory("LW-0000-0001"),
		{
			RepoName:      "lightwell/python/validated",
			AdvisoryID:    "x_DEMO-LW-0000-0002-4.0.0",
			PackageName:   "demo-pkg",
			FixedVersions: []string{"4.0.0"},
		},
	}}

	_, err := NewIngestor(jira, store, advisories).Sync(context.Background())
	require.NoError(t, err)
	require.Len(t, store.saved, 4)

	byKey := map[string]string{}
	for _, saved := range store.saved {
		byKey[saved.VulnerabilityKey] = saved.Stage
	}
	assert.Equal(t, "Lightwell Network", byKey["LTWL-1"])
	assert.Equal(t, "Lightwell Network", byKey["LTWL-2"])
	assert.Equal(t, "Validation", byKey["LTWL-3"])
	assert.Equal(t, "Fix in Progress", byKey["LTWL-4"])
}

func publishedJiraIssue(key, summary, status, lang, component, version string) jira_client.JiraIssue {
	issue := validJiraIssue(key)
	summaryJSON, err := json.Marshal(summary)
	if err != nil {
		panic(err)
	}
	statusJSON, err := json.Marshal(map[string]string{"name": status})
	if err != nil {
		panic(err)
	}
	labelsJSON, err := json.Marshal([]string{lang})
	if err != nil {
		panic(err)
	}
	issue.Fields["summary"] = summaryJSON
	issue.Fields["status"] = statusJSON
	issue.Fields["labels"] = labelsJSON
	issue.Fields["description"] = json.RawMessage(`{
		"type":"doc","content":[
			{"type":"paragraph","content":[{"type":"text","text":"Component: ` + component + `"}]},
			{"type":"paragraph","content":[{"type":"text","text":"version: ` + version + `"}]}
		]}`)
	return issue
}

func matchingJavaAdvisory(vulnID string) dao.LightwellAdvisoryInput {
	return dao.LightwellAdvisoryInput{
		RepoName:      "lightwell/java/remediated",
		AdvisoryID:    "x_DEMO-" + vulnID + "-1.2.3",
		PackageName:   "com.example:demo-lib",
		FixedVersions: []string{"1.2.3.build-00001"},
	}
}

func TestIngestorPaginatesAdvisories(t *testing.T) {
	rows := make([]dao.LightwellAdvisoryInput, advisoryPageSize+1)
	advisories := &countingAdvisories{fakeAdvisories: fakeAdvisories{rows: rows}}
	jira := &fakeJira{pages: map[string]jira_client.JiraPage{"": {Issues: []jira_client.JiraIssue{validJiraIssue("LTWL-1")}}}}

	_, err := NewIngestor(jira, &fakeVulnerabilityStore{outcome: dao.LightwellVulnerabilityInserted}, advisories).Sync(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 2, advisories.calls)
}

type countingAdvisories struct {
	fakeAdvisories
	calls int
}

func (c *countingAdvisories) List(ctx context.Context, offset int, limit int) ([]dao.LightwellAdvisoryInput, int64, error) {
	c.calls++
	return c.fakeAdvisories.List(ctx, offset, limit)
}

func TestIngestorAdvisoryListFailureWritesNothing(t *testing.T) {
	jira := &fakeJira{pages: map[string]jira_client.JiraPage{"": {Issues: []jira_client.JiraIssue{validJiraIssue("LTWL-1")}}}}
	store := &fakeVulnerabilityStore{outcome: dao.LightwellVulnerabilityInserted}

	_, err := NewIngestor(jira, store, fakeAdvisories{err: errors.New("list failed")}).Sync(context.Background())
	assert.ErrorContains(t, err, "list failed")
	assert.Empty(t, store.saved)
}
