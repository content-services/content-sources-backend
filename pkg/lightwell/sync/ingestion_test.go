package sync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/jira_client"
	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/tang/pkg/tangy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeJira struct {
	fields         []jira_client.JiraField
	fieldErr       error
	pages          map[string]jira_client.JiraPage
	searchErr      error
	batchSearchErr error
	issues         map[string]jira_client.JiraIssue
	issueCalls     []string
	issueFields    [][]string
	searchCalls    []string
	searchJQLs     []string
	searchFields   [][]string
}

func (f *fakeJira) Fields(context.Context) ([]jira_client.JiraField, error) {
	return f.fields, f.fieldErr
}

func (f *fakeJira) Search(_ context.Context, jql string, fields []string, token string) (jira_client.JiraPage, error) {
	f.searchCalls = append(f.searchCalls, token)
	f.searchJQLs = append(f.searchJQLs, jql)
	f.searchFields = append(f.searchFields, fields)
	if f.searchErr != nil {
		return jira_client.JiraPage{}, f.searchErr
	}
	if jql != VulnerabilityJQL {
		if f.batchSearchErr != nil {
			return jira_client.JiraPage{}, f.batchSearchErr
		}
		keys := make([]string, 0, len(f.issues))
		for key := range f.issues {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		page := jira_client.JiraPage{}
		for _, key := range keys {
			if strings.Contains(jql, `"`+key+`"`) {
				page.Issues = append(page.Issues, f.issues[key])
			}
		}
		return page, nil
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
	saved         []dao.LightwellVulnerabilityInput
	deleted       []string
	outcome       dao.LightwellVulnerabilitySaveOutcome
	deleteExisted bool
}

func (f *fakeVulnerabilityStore) Save(_ context.Context, input dao.LightwellVulnerabilityInput) (dao.LightwellVulnerabilitySaveOutcome, error) {
	f.saved = append(f.saved, input)
	return f.outcome, nil
}

func (f *fakeVulnerabilityStore) DeleteByKey(_ context.Context, key string) (bool, error) {
	f.deleted = append(f.deleted, key)
	return f.deleteExisted, nil
}

func TestIngestorBatchLoadsRelationships(t *testing.T) {
	first := validJiraIssue("LTWL-1")
	first.Fields["issuelinks"] = json.RawMessage(`[{"type":{"outward":"relates to"},"outwardIssue":{"key":"BATCH-1"}}]`)
	second := validJiraIssue("LTWL-2")
	second.Fields["issuelinks"] = json.RawMessage(`[{"type":{"outward":"relates to"},"outwardIssue":{"key":"BATCH-2"}}]`)
	jira := &fakeJira{
		fields: []jira_client.JiraField{{ID: "customfield_account", Name: "Account Number"}},
		pages:  map[string]jira_client.JiraPage{"": {Issues: []jira_client.JiraIssue{first, second}}},
		issues: map[string]jira_client.JiraIssue{
			"BATCH-1": {Key: "BATCH-1", Fields: map[string]json.RawMessage{"parent": json.RawMessage(`{"key":"EPIC-1"}`)}},
			"BATCH-2": {Key: "BATCH-2", Fields: map[string]json.RawMessage{"parent": json.RawMessage(`{"key":"EPIC-2"}`)}},
			"EPIC-1": {Key: "EPIC-1", Fields: map[string]json.RawMessage{
				"customfield_account": json.RawMessage(`["111"]`),
			}},
			"EPIC-2": {Key: "EPIC-2", Fields: map[string]json.RawMessage{
				"customfield_account": json.RawMessage(`["222"]`),
			}},
		},
	}
	store := &fakeVulnerabilityStore{outcome: dao.LightwellVulnerabilityInserted}

	summary, err := NewIngestor(jira, store, nil, nil).Sync(context.Background())

	require.NoError(t, err)
	assert.Empty(t, jira.issueCalls)
	assert.Equal(t, []string{
		VulnerabilityJQL,
		`key IN ("BATCH-1", "BATCH-2") ORDER BY key ASC`,
		`key IN ("EPIC-1", "EPIC-2") ORDER BY key ASC`,
	}, jira.searchJQLs)
	assert.Equal(t, 2, summary.Inserted)
	require.Len(t, store.saved, 2)
	assert.Equal(t, []dao.LightwellVulnerabilityTicket{{CustomerID: "111"}}, store.saved[0].Tickets)
	assert.Equal(t, []dao.LightwellVulnerabilityTicket{{CustomerID: "222"}}, store.saved[1].Tickets)
}

func TestIngestorChunksRelationshipBatchSearches(t *testing.T) {
	jira := &fakeJira{issues: make(map[string]jira_client.JiraIssue, relationshipBatchSize+1)}
	keys := make([]string, 0, relationshipBatchSize+1)
	for index := 0; index < relationshipBatchSize+1; index++ {
		key := fmt.Sprintf("BATCH-%03d", index)
		keys = append(keys, key)
		jira.issues[key] = jira_client.JiraIssue{Key: key}
	}
	ingestor := NewIngestor(jira, &fakeVulnerabilityStore{}, nil, nil)

	ingestor.prefetchIssues(context.Background(), keys, []string{"parent"})

	assert.Len(t, jira.searchJQLs, 2)
	assert.Len(t, ingestor.cache, relationshipBatchSize+1)
	assert.Contains(t, jira.searchJQLs[0], `"BATCH-099"`)
	assert.NotContains(t, jira.searchJQLs[0], `"BATCH-100"`)
	assert.Contains(t, jira.searchJQLs[1], `"BATCH-100"`)
}

func TestIngestorFallsBackWhenRelationshipBatchSearchFails(t *testing.T) {
	vulnerability := validJiraIssue("LTWL-1")
	vulnerability.Fields["issuelinks"] = json.RawMessage(`[{"type":{"outward":"relates to"},"outwardIssue":{"key":"BATCH-1"}}]`)
	jira := &fakeJira{
		fields:         []jira_client.JiraField{{ID: "customfield_account", Name: "Account Number"}},
		pages:          map[string]jira_client.JiraPage{"": {Issues: []jira_client.JiraIssue{vulnerability}}},
		batchSearchErr: errors.New("bulk search failed"),
		issues: map[string]jira_client.JiraIssue{
			"BATCH-1": {Key: "BATCH-1", Fields: map[string]json.RawMessage{"parent": json.RawMessage(`{"key":"EPIC-1"}`)}},
			"EPIC-1": {Key: "EPIC-1", Fields: map[string]json.RawMessage{
				"customfield_account": json.RawMessage(`["111"]`),
			}},
		},
	}
	store := &fakeVulnerabilityStore{outcome: dao.LightwellVulnerabilityInserted}

	summary, err := NewIngestor(jira, store, nil, nil).Sync(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 1, summary.Inserted)
	assert.Equal(t, []string{"BATCH-1", "EPIC-1"}, jira.issueCalls)
	require.Len(t, store.saved, 1)
	assert.Equal(t, []dao.LightwellVulnerabilityTicket{{CustomerID: "111"}}, store.saved[0].Tickets)
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

	summary, err := NewIngestor(jira, store, nil, nil).Sync(context.Background())
	require.NoError(t, err)
	assert.Equal(t, SyncSummary{Inserted: 1}, summary)
	require.Len(t, store.saved, 1)
	assert.Equal(t, "LTWL-1", store.saved[0].VulnerabilityKey)
	assert.Equal(t, "LW-0000-0001", store.saved[0].VulnerabilityID)
	assert.Equal(t, []dao.LightwellVulnerabilityTicket{{TicketID: "TICKET-1", CustomerID: "123"}}, store.saved[0].Tickets)
	assert.Equal(t, []string{"", "next", "", ""}, jira.searchCalls)
	assert.Empty(t, jira.issueCalls)
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

	summary, err := NewIngestor(jira, store, nil, nil).Sync(context.Background())
	require.NoError(t, err)
	assert.Equal(t, SyncSummary{Unchanged: 1}, summary)
	assert.Empty(t, jira.issueCalls)
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

	_, err := NewIngestor(jira, store, nil, nil).Sync(context.Background())
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

	_, err := NewIngestor(jira, store, nil, nil).Sync(context.Background())
	require.NoError(t, err)
	require.Len(t, store.saved, 1)
	assert.Equal(t, []string{"parent", "issuelinks"}, jira.searchFields[1])
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

	_, err := NewIngestor(jira, store, nil, nil).Sync(context.Background())
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

	summary, err := NewIngestor(jira, store, nil, nil).Sync(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, summary.Inserted)
	assert.Equal(t, 1, summary.Failed)
	assert.Len(t, summary.Failures, 1)
	assert.Len(t, store.saved, 1)
}

func TestIngestorSkipsDiscardedResolutions(t *testing.T) {
	for _, name := range []string{"Not a bug", "Duplicate", "Won't do"} {
		t.Run(name, func(t *testing.T) {
			issue := validJiraIssue("LTWL-1")
			issue.Fields["resolution"] = resolutionJSON(name)
			jira := &fakeJira{pages: map[string]jira_client.JiraPage{"": {Issues: []jira_client.JiraIssue{issue}}}}
			store := &fakeVulnerabilityStore{}

			summary, err := NewIngestor(jira, store, nil, nil).Sync(context.Background())
			require.NoError(t, err)
			assert.Equal(t, SyncSummary{}, summary)
			assert.Empty(t, store.saved)
			assert.Equal(t, []string{"LTWL-1"}, store.deleted)
		})
	}
}

func TestIngestorDeletesPreviouslyIngestedDiscardedIssue(t *testing.T) {
	issue := validJiraIssue("LTWL-1")
	issue.Fields["resolution"] = resolutionJSON("Duplicate")
	jira := &fakeJira{pages: map[string]jira_client.JiraPage{"": {Issues: []jira_client.JiraIssue{issue}}}}
	store := &fakeVulnerabilityStore{deleteExisted: true}

	summary, err := NewIngestor(jira, store, nil, nil).Sync(context.Background())
	require.NoError(t, err)
	assert.Equal(t, SyncSummary{Deleted: 1}, summary)
	assert.Empty(t, store.saved)
	assert.Equal(t, []string{"LTWL-1"}, store.deleted)
}

func TestIngestorSavesIssueWithOtherResolution(t *testing.T) {
	issue := validJiraIssue("LTWL-1")
	issue.Fields["resolution"] = resolutionJSON("Done")
	jira := &fakeJira{pages: map[string]jira_client.JiraPage{"": {Issues: []jira_client.JiraIssue{issue}}}}
	store := &fakeVulnerabilityStore{outcome: dao.LightwellVulnerabilityInserted}

	summary, err := NewIngestor(jira, store, nil, nil).Sync(context.Background())
	require.NoError(t, err)
	assert.Equal(t, SyncSummary{Inserted: 1}, summary)
	require.Len(t, store.saved, 1)
	assert.Empty(t, store.deleted)
}

func resolutionJSON(name string) json.RawMessage {
	raw, err := json.Marshal(map[string]string{"name": name})
	if err != nil {
		panic(err)
	}
	return raw
}

func TestIngestorSearchFailureWritesNothing(t *testing.T) {
	jira := &fakeJira{searchErr: errors.New("search failed")}
	store := &fakeVulnerabilityStore{outcome: dao.LightwellVulnerabilityInserted}

	_, err := NewIngestor(jira, store, nil, nil).Sync(context.Background())
	assert.ErrorContains(t, err, "search failed")
	assert.Empty(t, store.saved)
}

func TestIngestorRejectsAmbiguousAccountField(t *testing.T) {
	jira := &fakeJira{fields: []jira_client.JiraField{
		{ID: "customfield_1", Name: "Account Number"},
		{ID: "customfield_2", Name: "account number"},
	}}

	_, err := NewIngestor(jira, &fakeVulnerabilityStore{}, nil, nil).Sync(context.Background())
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

func TestIngestorPromotesEveryValidationStatusWhenPublished(t *testing.T) {
	advisories := fakeAdvisories{rows: []dao.LightwellAdvisoryInput{matchingJavaAdvisory("LW-0000-0001")}}
	for _, status := range []string{"Closed", "Verified", "Release Pending", "Released"} {
		t.Run(status, func(t *testing.T) {
			issue := publishedJiraIssue("LTWL-1", "LW-0000-0001 Example", status, "java", "com.example:demo-lib", "1.2.3")
			jira := &fakeJira{pages: map[string]jira_client.JiraPage{"": {Issues: []jira_client.JiraIssue{issue}}}}
			store := &fakeVulnerabilityStore{outcome: dao.LightwellVulnerabilityInserted}

			_, err := NewIngestor(jira, store, advisories, nil).Sync(context.Background())
			require.NoError(t, err)
			require.Len(t, store.saved, 1)
			assert.Equal(t, "Lightwell Network", store.saved[0].Stage)
			assert.Equal(t, []string{"1.2.3.build-00001"}, store.saved[0].PublishedVersions)
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

	_, err := NewIngestor(jira, store, advisories, nil).Sync(context.Background())
	require.NoError(t, err)
	require.Len(t, store.saved, 4)

	byKey := map[string]dao.LightwellVulnerabilityInput{}
	for _, saved := range store.saved {
		byKey[saved.VulnerabilityKey] = saved
	}
	assert.Equal(t, "Lightwell Network", byKey["LTWL-1"].Stage)
	assert.Equal(t, []string{"1.2.3.build-00001"}, byKey["LTWL-1"].PublishedVersions)
	assert.Equal(t, "Lightwell Network", byKey["LTWL-2"].Stage)
	assert.Equal(t, []string{"4.0.0"}, byKey["LTWL-2"].PublishedVersions)
	assert.Equal(t, "Validation", byKey["LTWL-3"].Stage)
	assert.Empty(t, byKey["LTWL-3"].PublishedVersions)
	assert.Equal(t, "Fix in Progress", byKey["LTWL-4"].Stage)
	assert.Empty(t, byKey["LTWL-4"].PublishedVersions)
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

	_, err := NewIngestor(jira, &fakeVulnerabilityStore{outcome: dao.LightwellVulnerabilityInserted}, advisories, nil).Sync(context.Background())
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

	_, err := NewIngestor(jira, store, fakeAdvisories{err: errors.New("list failed")}, nil).Sync(context.Background())
	assert.ErrorContains(t, err, "list failed")
	assert.Empty(t, store.saved)
}

type fakePublicationVerifier struct {
	confirmed map[PublicationPackage][]string
	warnings  []string
	calls     int
}

func (f *fakePublicationVerifier) VerifyPublished(
	_ context.Context,
	_ map[PublicationPackage][]string,
) (map[PublicationPackage][]string, []string) {
	f.calls++
	return f.confirmed, f.warnings
}

func TestIngestorPublicationFlow(t *testing.T) {
	t.Run("confirmed breadcrumbs merge with advisories", func(t *testing.T) {
		ctx := context.Background()
		issue := validationIssueWithBreadcrumbs(t, "LTWL-1",
			"pkg:maven/org.example/demo@1.2.3.rhlw-00002",
			"pkg:maven/org.example/demo@9.9.9",
			"pkg:pypi/zope.interface@6.0-rhlw-00001",
		)
		jira := &fakeJira{pages: map[string]jira_client.JiraPage{"": {Issues: []jira_client.JiraIssue{issue}}}}
		store := &fakeVulnerabilityStore{outcome: dao.LightwellVulnerabilityInserted}
		advisories := fakeAdvisories{rows: []dao.LightwellAdvisoryInput{{
			RepoName: "java/remediated", AdvisoryID: "LW-0000-0001-1.2.3",
			PackageName: "org.example:demo", FixedVersions: []string{"1.2.3.rhlw-00001", "1.2.3.rhlw-00002"},
		}}}
		repositories := mockPublicationRepositories(t, ctx, []api.RepositoryResponse{
			{PublishedDistBasePath: "java/remediated", ContentType: "maven"},
			{PublishedDistBasePath: "python/predisclosure", ContentType: "python"},
		})
		resolver := pulp_client.NewMockPulpClient(t)
		expectPublicationRepository(resolver, ctx, "java/remediated")
		expectPublicationRepository(resolver, ctx, "python/predisclosure")
		tang := tangy.NewMockTangy(t)
		tang.On("MavenVersionsList", ctx, "java/remediated-href", "org.example", "demo", "", tangy.PageOptions{Limit: 1000}).Return(tangy.MavenVersionsResponse{
			Results: []tangy.MavenVersionsItem{{
				Version: "1.2.3",
				Builds:  []tangy.MavenBuildInfo{{Version: "1.2.3", Release: "rhlw-00002"}},
			}},
			Total: 1,
		}, nil).Once()
		tang.On("PythonPackageVersionsGet", ctx, "python/predisclosure-href", "zope-interface").Return([]tangy.PythonPackageDetail{{
			Version: "6.0-rhlw-00001",
		}}, nil).Once()
		verifier := NewPublicationPackageVerifier(repositories, resolver, tang)

		summary, err := NewIngestor(jira, store, advisories, verifier).Sync(ctx)

		require.NoError(t, err)
		assert.Equal(t, 1, summary.Inserted)
		assert.Zero(t, summary.Failed)
		assert.Empty(t, summary.Warnings)
		require.Len(t, store.saved, 1)
		assert.Equal(t, "Lightwell Network", store.saved[0].Stage)
		assert.Equal(t, []string{"6.0-rhlw-00001", "1.2.3.rhlw-00002", "1.2.3.rhlw-00001"}, store.saved[0].PublishedVersions)
	})

	t.Run("verification error falls back to advisories", func(t *testing.T) {
		ctx := context.Background()
		issue := validationIssueWithBreadcrumbs(t, "LTWL-1", "pkg:maven/org.example/demo@1.2.3.rhlw-00002")
		jira := &fakeJira{pages: map[string]jira_client.JiraPage{"": {Issues: []jira_client.JiraIssue{issue}}}}
		store := &fakeVulnerabilityStore{outcome: dao.LightwellVulnerabilityInserted}
		advisories := fakeAdvisories{rows: []dao.LightwellAdvisoryInput{{
			RepoName: "java/remediated", AdvisoryID: "LW-0000-0001-1.2.3",
			PackageName: "org.example:demo", FixedVersions: []string{"1.2.3.advisory-00001"},
		}}}
		repositories := mockPublicationRepositories(t, ctx, []api.RepositoryResponse{{
			PublishedDistBasePath: "java/remediated", ContentType: "maven",
		}})
		resolver := pulp_client.NewMockPulpClient(t)
		expectPublicationRepository(resolver, ctx, "java/remediated")
		tang := tangy.NewMockTangy(t)
		tang.On("MavenVersionsList", ctx, "java/remediated-href", "org.example", "demo", "", tangy.PageOptions{Limit: 1000}).
			Return(tangy.MavenVersionsResponse{}, errors.New("Tang unavailable")).Once()
		verifier := NewPublicationPackageVerifier(repositories, resolver, tang)

		summary, err := NewIngestor(jira, store, advisories, verifier).Sync(ctx)

		require.NoError(t, err)
		assert.Equal(t, 1, summary.Inserted)
		assert.Zero(t, summary.Failed)
		require.Len(t, summary.Warnings, 1)
		assert.Contains(t, summary.Warnings[0], "Tang unavailable")
		require.Len(t, store.saved, 1)
		assert.Equal(t, "Lightwell Network", store.saved[0].Stage)
		assert.Equal(t, []string{"1.2.3.advisory-00001"}, store.saved[0].PublishedVersions)
	})
}

func TestIngestorCommonsIOBareBreadcrumbsUseAdvisories(t *testing.T) {
	pkg := PublicationPackage{Ecosystem: "maven", Namespace: "commons-io", Name: "commons-io"}
	for _, test := range []struct {
		key             string
		breadcrumb      []string
		advisoryVersion string
		verifierCalls   int
	}{
		{key: "LTWL-1837", breadcrumb: []string{"pkg:maven/commons-io/commons-io@2.11.0"}, advisoryVersion: "2.11.0.rhlw-00000-n-00008"},
		{key: "LTWL-1839", breadcrumb: []string{"pkg:maven/commons-io/commons-io@2.11.0"}, advisoryVersion: "2.11.0.rhlw-00000-n-00008"},
		{key: "LTWL-1840", breadcrumb: []string{"pkg:maven/commons-io/commons-io@2.11.0"}, advisoryVersion: "2.11.0.rhlw-00000-n-00008"},
		{key: "LTWL-2147", breadcrumb: []string{"pkg:maven/commons-io/commons-io@2.11.0"}, advisoryVersion: "2.11.0.rhlw-00000-n-00008"},
		{key: "LTWL-1106", breadcrumb: []string{"pkg:maven/commons-io/commons-io@2.11.0", "pkg:maven/commons-io/commons-io@2.11.0.rhlw-00009"}, advisoryVersion: "2.11.0.rhlw-00009", verifierCalls: 1},
	} {
		t.Run(test.key, func(t *testing.T) {
			issue := validationIssueWithBreadcrumbs(t, test.key, test.breadcrumb...)
			issue.Fields["description"] = jsonString(t, "Component: commons-io:commons-io\nversion: 2.11.0\nLightwell Fixed Versions:\n"+strings.Join(test.breadcrumb, "\n"))
			jira := &fakeJira{pages: map[string]jira_client.JiraPage{"": {Issues: []jira_client.JiraIssue{issue}}}}
			store := &fakeVulnerabilityStore{outcome: dao.LightwellVulnerabilityInserted}
			advisories := fakeAdvisories{rows: []dao.LightwellAdvisoryInput{{
				RepoName: "java/remediated", AdvisoryID: "LW-0000-0001-2.11.0",
				PackageName: "commons-io:commons-io", FixedVersions: []string{test.advisoryVersion},
			}}}
			verifier := &fakePublicationVerifier{confirmed: map[PublicationPackage][]string{
				pkg: {"2.11.0.rhlw-00009"},
			}}

			_, err := NewIngestor(jira, store, advisories, verifier).Sync(context.Background())

			require.NoError(t, err)
			assert.Equal(t, test.verifierCalls, verifier.calls)
			require.Len(t, store.saved, 1)
			assert.Equal(t, "Lightwell Network", store.saved[0].Stage)
			assert.Equal(t, []string{test.advisoryVersion}, store.saved[0].PublishedVersions)
		})
	}
}

func TestIngestorFallsBackToAdvisoriesForMissingBreadcrumb(t *testing.T) {
	issue := validationIssueWithBreadcrumbs(t, "LTWL-1", "pkg:maven/org.example/demo@1.2.3.rhlw-00001")
	jira := &fakeJira{pages: map[string]jira_client.JiraPage{"": {Issues: []jira_client.JiraIssue{issue}}}}
	store := &fakeVulnerabilityStore{outcome: dao.LightwellVulnerabilityInserted}
	advisories := fakeAdvisories{rows: []dao.LightwellAdvisoryInput{{
		RepoName: "java/remediated", AdvisoryID: "LW-0000-0001-1.2.3",
		PackageName: "org.example:demo", FixedVersions: []string{"1.2.3.advisory-00001"},
	}}}

	summary, err := NewIngestor(jira, store, advisories, &fakePublicationVerifier{}).Sync(context.Background())

	require.NoError(t, err)
	require.Len(t, store.saved, 1)
	assert.Equal(t, "Lightwell Network", store.saved[0].Stage)
	assert.Equal(t, []string{"1.2.3.advisory-00001"}, store.saved[0].PublishedVersions)
	assert.Zero(t, summary.Failed)
}

func TestIngestorKeepsValidationWhenNeitherPublicationSignalMatches(t *testing.T) {
	issue := validationIssueWithBreadcrumbs(t, "LTWL-1", "pkg:maven/org.example/demo@1.2.3.rhlw-00001")
	jira := &fakeJira{pages: map[string]jira_client.JiraPage{"": {Issues: []jira_client.JiraIssue{issue}}}}
	store := &fakeVulnerabilityStore{outcome: dao.LightwellVulnerabilityInserted}

	_, err := NewIngestor(jira, store, fakeAdvisories{}, &fakePublicationVerifier{}).Sync(context.Background())

	require.NoError(t, err)
	require.Len(t, store.saved, 1)
	assert.Equal(t, "Validation", store.saved[0].Stage)
	assert.Empty(t, store.saved[0].PublishedVersions)
}

func TestIngestorDoesNotVerifyNonValidationOrDiscardedIssues(t *testing.T) {
	inProgress := validationIssueWithBreadcrumbs(t, "LTWL-1", "pkg:maven/org.example/demo@1.0.0")
	inProgress.Fields["status"] = json.RawMessage(`{"name":"In Progress"}`)
	discarded := validationIssueWithBreadcrumbs(t, "LTWL-2", "pkg:maven/org.example/demo@2.0.0")
	discarded.Fields["resolution"] = json.RawMessage(`{"name":"Duplicate"}`)
	jira := &fakeJira{pages: map[string]jira_client.JiraPage{"": {Issues: []jira_client.JiraIssue{inProgress, discarded}}}}
	store := &fakeVulnerabilityStore{outcome: dao.LightwellVulnerabilityInserted}
	verifier := &fakePublicationVerifier{}

	_, err := NewIngestor(jira, store, nil, verifier).Sync(context.Background())

	require.NoError(t, err)
	assert.Zero(t, verifier.calls)
	require.Len(t, store.saved, 1)
	assert.Equal(t, "Fix in Progress", store.saved[0].Stage)
}

func TestIngestorVerificationWarningsDoNotPreventOtherIssuesFromSaving(t *testing.T) {
	first := validationIssueWithBreadcrumbs(t, "LTWL-1", "pkg:maven/org.example/demo@1.0.0.rhlw-00001")
	second := validationIssueWithBreadcrumbs(t, "LTWL-2", "pkg:pypi/demo@2.0.0-rhlw-00001")
	jira := &fakeJira{pages: map[string]jira_client.JiraPage{"": {Issues: []jira_client.JiraIssue{first, second}}}}
	store := &fakeVulnerabilityStore{outcome: dao.LightwellVulnerabilityInserted}
	verifier := &fakePublicationVerifier{
		confirmed: map[PublicationPackage][]string{
			{Ecosystem: "pypi", Name: "demo"}: {"2.0.0-rhlw-00001"},
		},
		warnings: []string{"Maven repository unavailable"},
	}

	summary, err := NewIngestor(jira, store, nil, verifier).Sync(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 2, summary.Inserted)
	assert.Zero(t, summary.Failed)
	assert.Equal(t, []string{"Maven repository unavailable"}, summary.Warnings)
	require.Len(t, store.saved, 2)
	assert.Equal(t, "Validation", store.saved[0].Stage)
	assert.Equal(t, "Lightwell Network", store.saved[1].Stage)
}

func validationIssueWithBreadcrumbs(t *testing.T, key string, purls ...string) jira_client.JiraIssue {
	t.Helper()
	issue := validJiraIssue(key)
	issue.Fields["status"] = json.RawMessage(`{"name":"Closed"}`)
	issue.Fields["labels"] = json.RawMessage(`["java"]`)
	description := "Component: org.example:demo\nversion: 1.2.3\nLightwell Fixed Versions:\n" + strings.Join(purls, "\n")
	descriptionJSON, err := json.Marshal(description)
	require.NoError(t, err)
	issue.Fields["description"] = descriptionJSON
	return issue
}
