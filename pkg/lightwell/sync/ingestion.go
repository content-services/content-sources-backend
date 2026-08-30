package sync

import (
	"context"
	"fmt"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/clients/jira_client"
	"github.com/content-services/content-sources-backend/pkg/dao"
)

const VulnerabilityJQL = "project = LTWL AND type = Vulnerability ORDER BY created ASC"

var vulnerabilityFields = []string{
	"summary",
	"description",
	"status",
	"created",
	"updated",
	"resolution",
	"labels",
	"issuelinks",
	fieldPURL,
	fieldCWE,
	fieldSeverity,
	fieldCVSS,
	fieldEmbargo,
}

type SyncSummary struct {
	Inserted  int
	Updated   int
	Unchanged int
	Deleted   int
	Failed    int
	// Failures are per-issue problems encountered while the sync still completed.
	// A non-nil Sync error means the job could not run at all.
	Failures []string
}

// VulnerabilityStore persists the vulnerabilities discovered during ingestion.
// It is the narrow subset of dao.LightwellVulnerabilityDao that the Ingestor
// actually uses, which keeps the constructor easy to fake in tests.
type VulnerabilityStore interface {
	Save(ctx context.Context, input dao.LightwellVulnerabilityInput) (dao.LightwellVulnerabilitySaveOutcome, error)
	DeleteByKey(ctx context.Context, vulnerabilityKey string) (bool, error)
}

// AdvisoryStore is the subset of dao.LightwellAdvisoryDao used to decide
// whether a vulnerability has been published on the Lightwell Network.
type AdvisoryStore interface {
	List(ctx context.Context, offset int, limit int) ([]dao.LightwellAdvisoryInput, int64, error)
}

type Ingestor struct {
	jira            jira_client.JiraClient
	vulnerabilities VulnerabilityStore
	advisories      AdvisoryStore
	cache           map[string]cachedIssue
}

type cachedIssue struct {
	issue jira_client.JiraIssue
	err   error
}

func NewIngestor(jira jira_client.JiraClient, vulnerabilities VulnerabilityStore, advisories AdvisoryStore) *Ingestor {
	return &Ingestor{jira: jira, vulnerabilities: vulnerabilities, advisories: advisories, cache: make(map[string]cachedIssue)}
}

func (i *Ingestor) Sync(ctx context.Context) (SyncSummary, error) {
	accountFieldID, err := i.accountFieldID(ctx)
	if err != nil {
		return SyncSummary{}, err
	}

	issues, err := i.searchAll(ctx)
	if err != nil {
		return SyncSummary{}, err
	}

	advisories, err := i.loadAdvisories(ctx)
	if err != nil {
		return SyncSummary{}, err
	}

	var summary SyncSummary
	for _, issue := range issues {
		if err := i.syncIssue(ctx, issue, accountFieldID, advisories, &summary); err != nil {
			summary.Failed++
			summary.Failures = append(summary.Failures, err.Error())
		}
	}
	return summary, nil
}

const advisoryPageSize = 100

func (i *Ingestor) loadAdvisories(ctx context.Context) ([]PublishedAdvisory, error) {
	if i.advisories == nil {
		return nil, nil
	}
	var advisories []PublishedAdvisory
	for offset := 0; ; offset += advisoryPageSize {
		rows, total, err := i.advisories.List(ctx, offset, advisoryPageSize)
		if err != nil {
			return nil, fmt.Errorf("list advisories: %w", err)
		}
		for _, row := range rows {
			advisories = append(advisories, PublishedAdvisory{
				RepoName:      row.RepoName,
				AdvisoryID:    row.AdvisoryID,
				PackageName:   row.PackageName,
				FixedVersions: row.FixedVersions,
			})
		}
		if len(rows) == 0 || offset+len(rows) >= int(total) {
			return advisories, nil
		}
	}
}

func (i *Ingestor) searchAll(ctx context.Context) ([]jira_client.JiraIssue, error) {
	issues := make([]jira_client.JiraIssue, 0)
	nextPageToken := ""
	seenTokens := make(map[string]struct{})
	for {
		page, err := i.jira.Search(ctx, VulnerabilityJQL, vulnerabilityFields, nextPageToken)
		if err != nil {
			return nil, err
		}
		issues = append(issues, page.Issues...)
		if page.NextPageToken == "" {
			return issues, nil
		}
		if _, seen := seenTokens[page.NextPageToken]; seen {
			return nil, fmt.Errorf("jira repeated pagination token %q", page.NextPageToken)
		}
		seenTokens[page.NextPageToken] = struct{}{}
		nextPageToken = page.NextPageToken
	}
}

func (i *Ingestor) accountFieldID(ctx context.Context) (string, error) {
	fields, err := i.jira.Fields(ctx)
	if err != nil {
		return "", err
	}
	matches := make([]string, 0, 1)
	for _, field := range fields {
		if strings.EqualFold(strings.TrimSpace(field.Name), "Account Number") {
			matches = append(matches, field.ID)
		}
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("multiple Jira fields named Account Number: %s", strings.Join(matches, ", "))
	}
	if len(matches) == 0 {
		return "", nil
	}
	return matches[0], nil
}

func (i *Ingestor) syncIssue(ctx context.Context, issue jira_client.JiraIssue, accountFieldID string, advisories []PublishedAdvisory, summary *SyncSummary) error {
	if discardedResolution(issue.Fields["resolution"]) {
		return i.deleteDiscardedIssue(ctx, issue.Key, summary)
	}

	vulnerability, err := mapVulnerability(issue)
	if err != nil {
		return err
	}

	tickets, err := i.issueRelationships(ctx, issue, accountFieldID)
	if err != nil {
		return fmt.Errorf("issue %s relationships: %w", issue.Key, err)
	}

	applyPublishedStage(&vulnerability, advisories)

	outcome, err := i.vulnerabilities.Save(ctx, vulnerabilityInput(vulnerability, tickets))
	if err != nil {
		return fmt.Errorf("issue %s database save: %w", issue.Key, err)
	}
	switch outcome {
	case dao.LightwellVulnerabilityInserted:
		summary.Inserted++
	case dao.LightwellVulnerabilityUpdated:
		summary.Updated++
	default:
		summary.Unchanged++
	}
	return nil
}

func (i *Ingestor) deleteDiscardedIssue(ctx context.Context, key string, summary *SyncSummary) error {
	if key == "" {
		return fmt.Errorf("issue has no key")
	}
	deleted, err := i.vulnerabilities.DeleteByKey(ctx, key)
	if err != nil {
		return fmt.Errorf("issue %s database delete: %w", key, err)
	}
	if deleted {
		summary.Deleted++
	}
	return nil
}

// vulnerabilityInput converts the mapped vulnerability and its ticket links into
// the persistence-facing input consumed by the DAO.
func vulnerabilityInput(vulnerability Vulnerability, tickets []TicketLink) dao.LightwellVulnerabilityInput {
	input := dao.LightwellVulnerabilityInput{
		VulnerabilityKey:   vulnerability.VulnerabilityKey,
		VulnerabilityID:    vulnerability.VulnerabilityID,
		PURL:               vulnerability.PURL,
		ComponentName:      vulnerability.ComponentName,
		ComponentVersion:   vulnerability.ComponentVersion,
		Title:              vulnerability.Title,
		CWE:                vulnerability.CWE,
		Description:        vulnerability.Description,
		Severity:           vulnerability.Severity,
		CVSS:               vulnerability.CVSS,
		CVSSVector:         vulnerability.CVSSVector,
		ExploitTested:      vulnerability.ExploitTested,
		ReproducerIncluded: vulnerability.ReproducerIncluded,
		CustomerPriority:   vulnerability.CustomerPriority,
		Stage:              vulnerability.Stage,
		Language:           vulnerability.Language,
		Complexity:         vulnerability.Complexity,
		SubmittedDate:      vulnerability.SubmittedDate,
		LastUpdated:        vulnerability.LastUpdated,
		Embargo:            vulnerability.Embargo,
		Duplicate:          vulnerability.Duplicate,
	}
	for _, ticket := range tickets {
		input.Tickets = append(input.Tickets, dao.LightwellVulnerabilityTicket{
			TicketID:   ticket.TicketID,
			CustomerID: ticket.CustomerID,
		})
	}
	return input
}

func applyPublishedStage(vulnerability *Vulnerability, advisories []PublishedAdvisory) {
	if vulnerability.Stage != "Validation" {
		return
	}
	if publishedOnNetwork(*vulnerability, advisories) {
		vulnerability.Stage = "Lightwell Network"
	}
}

// issueRelationships resolves the support-ticket links for a vulnerability. A
// vulnerability can relate to several batch tickets, each of which rolls up to
// an epic. The epic maps to one customer account, and the stored ticket id is
// the issue that epic is triggered by. This walks every "relates to" link and
// returns one TicketLink per distinct epic.
func (i *Ingestor) issueRelationships(ctx context.Context, vulnerability jira_client.JiraIssue, accountFieldID string) ([]TicketLink, error) {
	if accountFieldID == "" {
		return nil, nil
	}
	batchKeys := linkedIssueKeys(vulnerability, "relates to")
	if len(batchKeys) == 0 {
		return nil, nil
	}

	tickets := make([]TicketLink, 0, len(batchKeys))
	seenEpics := make(map[string]struct{}, len(batchKeys))
	for _, batchKey := range batchKeys {
		batch, err := i.issue(ctx, batchKey, []string{"parent", "issuelinks"})
		if err != nil {
			return nil, err
		}

		epicKey := parentIssueKey(batch)
		if epicKey == "" {
			epicKey = linkedIssueKey(batch, "is child of")
		}
		if epicKey == "" {
			continue
		}
		if _, seen := seenEpics[epicKey]; seen {
			continue
		}
		seenEpics[epicKey] = struct{}{}

		epic, err := i.issue(ctx, epicKey, []string{accountFieldID, "issuelinks"})
		if err != nil {
			return nil, err
		}
		ticketID := linkedIssueKey(epic, "is triggered by")
		// A support ticket (epic) maps to exactly one customer account, so the
		// first account number on the epic identifies the customer even when no
		// support ticket is linked.
		customers := normalizedValues(epic.Fields[accountFieldID])
		if len(customers) == 0 {
			continue
		}
		tickets = append(tickets, TicketLink{TicketID: ticketID, CustomerID: customers[0]})
	}
	return tickets, nil
}

func (i *Ingestor) issue(ctx context.Context, key string, fields []string) (jira_client.JiraIssue, error) {
	if cached, exists := i.cache[key]; exists {
		return cached.issue, cached.err
	}
	issue, err := i.jira.Issue(ctx, key, fields)
	i.cache[key] = cachedIssue{issue: issue, err: err}
	return issue, err
}
