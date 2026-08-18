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

	summary, err := NewIngestor(jira, store).Sync(context.Background())
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

	summary, err := NewIngestor(jira, store).Sync(context.Background())
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

	_, err := NewIngestor(jira, store).Sync(context.Background())
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

	_, err := NewIngestor(jira, store).Sync(context.Background())
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

	_, err := NewIngestor(jira, store).Sync(context.Background())
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

	summary, err := NewIngestor(jira, store).Sync(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, summary.Inserted)
	assert.Equal(t, 1, summary.Failed)
	assert.Len(t, summary.Failures, 1)
	assert.Len(t, store.saved, 1)
}

func TestIngestorSearchFailureWritesNothing(t *testing.T) {
	jira := &fakeJira{searchErr: errors.New("search failed")}
	store := &fakeVulnerabilityStore{outcome: dao.LightwellVulnerabilityInserted}

	_, err := NewIngestor(jira, store).Sync(context.Background())
	assert.ErrorContains(t, err, "search failed")
	assert.Empty(t, store.saved)
}

func TestIngestorRejectsAmbiguousAccountField(t *testing.T) {
	jira := &fakeJira{fields: []jira_client.JiraField{
		{ID: "customfield_1", Name: "Account Number"},
		{ID: "customfield_2", Name: "account number"},
	}}

	_, err := NewIngestor(jira, &fakeVulnerabilityStore{}).Sync(context.Background())
	assert.ErrorContains(t, err, "multiple Jira fields")
}
