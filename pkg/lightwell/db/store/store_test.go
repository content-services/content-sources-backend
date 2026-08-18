package store_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/lightwell/db/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	config.ConfigureLogging()
	dbConfig := config.Get().Database
	connStr := fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=disable",
		dbConfig.User,
		dbConfig.Password,
		dbConfig.Host,
		dbConfig.Port,
		dbConfig.Name,
	)

	pool, err := pgxpool.New(context.Background(), connStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to connect to database: %v\n", err)
		os.Exit(1)
	}
	testPool = pool

	exitCode := m.Run()
	pool.Close()
	os.Exit(exitCode)
}

type testVulnSpec struct {
	vulnID      string
	severity    string
	stage       string
	language    string
	complexity  string
	ticketID    string
	ticketIDs   []string
	daysAgo     int
	embargo     bool
	duplicate   bool
	duplicateOf *string
	component   string
	title       string
	customerIDs []string
}

func beginTestTx(t *testing.T) (context.Context, pgx.Tx, *store.Queries) {
	ctx := context.Background()
	tx, err := testPool.Begin(ctx)
	require.NoError(t, err)
	return ctx, tx, store.New(tx)
}

func rollbackTestTx(t *testing.T, tx pgx.Tx) {
	require.NoError(t, tx.Rollback(context.Background()))
}

func insertTestVulnerabilities(t *testing.T, ctx context.Context, tx pgx.Tx, specs []testVulnSpec) map[string]uuid.UUID {
	ids := make(map[string]uuid.UUID, len(specs))
	for _, spec := range specs {
		id := uuid.New()
		ids[spec.vulnID] = id
		_, err := tx.Exec(ctx, `
			INSERT INTO lightwell_vulnerabilities (
				uuid, vulnerability_key, vulnerability_id, component_name, component_version, title, severity, stage, language, complexity,
				submitted_date, last_updated, embargo, duplicate, duplicate_of
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, CURRENT_DATE - $11::int, NOW(), $12, $13, $14
			)`,
			id,
			spec.vulnID,
			spec.vulnID,
			spec.component,
			"1.0.0",
			spec.title,
			spec.severity,
			spec.stage,
			spec.language,
			spec.complexity,
			spec.daysAgo,
			spec.embargo,
			spec.duplicate,
			spec.duplicateOf,
		)
		require.NoError(t, err)

		for _, customerID := range spec.customerIDs {
			_, err = tx.Exec(ctx,
				`INSERT INTO lightwell_vulnerability_customers (customer_id, vulnerability_uuid) VALUES ($1, $2)`,
				customerID, id,
			)
			require.NoError(t, err)
		}

		ticketIDs := spec.ticketIDs
		if len(ticketIDs) == 0 && spec.ticketID != "" {
			ticketIDs = []string{spec.ticketID}
		}
		ticketCustomerID := ""
		if len(spec.customerIDs) > 0 {
			ticketCustomerID = spec.customerIDs[0]
		}
		for _, ticketID := range ticketIDs {
			_, err = tx.Exec(ctx,
				`INSERT INTO lightwell_vulnerability_support_tickets (vulnerability_uuid, customer_id, ticket_id) VALUES ($1, $2, $3)`,
				id, ticketCustomerID, ticketID,
			)
			require.NoError(t, err)
		}
	}
	return ids
}

func filterParams(customerID string) store.CountAggregatesParams {
	return store.CountAggregatesParams{
		CustomerID: customerID,
	}
}

func listParams(customerID string) store.ListVulnerabilitiesParams {
	return store.ListVulnerabilitiesParams{
		CustomerID: customerID,
		PageLimit:  100,
	}
}

func stageParams(customerID string) store.CountByStageParams {
	return store.CountByStageParams{
		CustomerID: customerID,
	}
}

func textFlag(value string) *string {
	return &value
}

func textSearch(value string) *string {
	return &value
}

func vulnIDs(rows []store.ListVulnerabilitiesRow) []string {
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.VulnerabilityID)
	}
	return ids
}

func TestStore_CustomerScopingAndFilters(t *testing.T) {
	ctx, tx, q := beginTestTx(t)
	defer rollbackTestTx(t, tx)

	customerA := fmt.Sprintf("lw-scope-a-%d", time.Now().UnixNano())
	customerB := fmt.Sprintf("lw-scope-b-%d", time.Now().UnixNano())

	specs := []testVulnSpec{
		{
			vulnID:      "LWL-TEST-CRIT-STANDARD",
			severity:    "Critical",
			stage:       "Submitted",
			language:    "java",
			complexity:  "Standard",
			ticketIDs:   []string{"ticket-alpha", "ticket-shared"},
			daysAgo:     8,
			embargo:     true,
			component:   "log4j-core",
			title:       "JNDI injection test",
			customerIDs: []string{customerA},
		},
		{
			vulnID:      "LWL-TEST-IMP-COMPLEX",
			severity:    "Important",
			stage:       "Submitted",
			language:    "java",
			complexity:  "Complex",
			ticketID:    "ticket-alpha",
			daysAgo:     10,
			component:   "spring-web",
			title:       "HTTP smuggling test",
			customerIDs: []string{customerA},
		},
		{
			vulnID:      "LWL-TEST-MOD-COMPLEX",
			severity:    "Moderate",
			stage:       "Classified",
			language:    "python",
			complexity:  "Complex",
			ticketID:    "ticket-beta",
			daysAgo:     16,
			component:   "requests",
			title:       "SSRF redirect test",
			customerIDs: []string{customerA},
		},
		{
			vulnID:      "LWL-TEST-CRIT-EXTENSIVE",
			severity:    "Critical",
			stage:       "Fix in Progress",
			language:    "java",
			complexity:  "Extensive",
			ticketID:    "ticket-beta",
			daysAgo:     31,
			component:   "jackson-databind",
			title:       "Deserialization gadget test",
			customerIDs: []string{customerA},
		},
		{
			vulnID:      "LWL-TEST-NETWORK",
			severity:    "Important",
			stage:       "Lightwell Network",
			language:    "javascript",
			complexity:  "Extensive",
			ticketID:    "ticket-alpha",
			daysAgo:     50,
			component:   "express",
			title:       "Path traversal test",
			customerIDs: []string{customerA},
		},
		{
			vulnID:      "LWL-TEST-DUP",
			severity:    "Moderate",
			stage:       "Validation",
			language:    "javascript",
			complexity:  "Standard",
			ticketID:    "ticket-beta",
			daysAgo:     20,
			duplicate:   true,
			duplicateOf: textFlag("LWL-TEST-CRIT-STANDARD"),
			component:   "lodash",
			title:       "Prototype pollution test",
			customerIDs: []string{customerA},
		},
		{
			vulnID:      "LWL-TEST-OTHER-CUSTOMER",
			severity:    "Critical",
			stage:       "Submitted",
			language:    "java",
			complexity:  "Standard",
			ticketID:    "ticket-other",
			daysAgo:     9,
			component:   "other-package",
			title:       "Other customer only",
			customerIDs: []string{customerB},
		},
	}
	insertTestVulnerabilities(t, ctx, tx, specs)

	allForA, err := q.ListVulnerabilities(ctx, listParams(customerA))
	require.NoError(t, err)
	assert.Len(t, allForA, 6)
	assert.False(t, allForA[0].LastUpdated.IsZero())

	allForB, err := q.ListVulnerabilities(ctx, listParams(customerB))
	require.NoError(t, err)
	assert.Len(t, allForB, 1)
	assert.Equal(t, "LWL-TEST-OTHER-CUSTOMER", allForB[0].VulnerabilityID)

	criticalParams := listParams(customerA)
	criticalParams.Severities = []string{"Critical"}
	critical, err := q.ListVulnerabilities(ctx, criticalParams)
	require.NoError(t, err)
	assert.Len(t, critical, 2)

	stageFilter := listParams(customerA)
	stageFilter.Stages = []string{"Classified"}
	classified, err := q.ListVulnerabilities(ctx, stageFilter)
	require.NoError(t, err)
	assert.Len(t, classified, 1)
	assert.Equal(t, "LWL-TEST-MOD-COMPLEX", classified[0].VulnerabilityID)

	complexityParams := listParams(customerA)
	complexityParams.Complexities = []string{"Standard"}
	standard, err := q.ListVulnerabilities(ctx, complexityParams)
	require.NoError(t, err)
	assert.Len(t, standard, 2)

	ticketParams := listParams(customerA)
	ticketParams.LtwlsuptTicketIds = []string{"ticket-beta"}
	beta, err := q.ListVulnerabilities(ctx, ticketParams)
	require.NoError(t, err)
	assert.Len(t, beta, 3)

	sharedTicket := listParams(customerA)
	sharedTicket.LtwlsuptTicketIds = []string{"ticket-shared"}
	shared, err := q.ListVulnerabilities(ctx, sharedTicket)
	require.NoError(t, err)
	require.Len(t, shared, 1)
	assert.Equal(t, "LWL-TEST-CRIT-STANDARD", shared[0].VulnerabilityID)
	assert.Equal(t, []string{"ticket-alpha", "ticket-shared"}, shared[0].LtwlsuptTicketIds)

	ticketOr := listParams(customerA)
	ticketOr.LtwlsuptTicketIds = []string{"ticket-shared", "ticket-beta"}
	ticketOrRows, err := q.ListVulnerabilities(ctx, ticketOr)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{
		"LWL-TEST-CRIT-STANDARD",
		"LWL-TEST-MOD-COMPLEX",
		"LWL-TEST-CRIT-EXTENSIVE",
		"LWL-TEST-DUP",
	}, vulnIDs(ticketOrRows))

	embargoParams := listParams(customerA)
	embargoParams.Flags = []string{"embargo"}
	embargoed, err := q.ListVulnerabilities(ctx, embargoParams)
	require.NoError(t, err)
	assert.Len(t, embargoed, 1)
	assert.Equal(t, "LWL-TEST-CRIT-STANDARD", embargoed[0].VulnerabilityID)

	dupParams := listParams(customerA)
	dupParams.Flags = []string{"duplicate"}
	duplicates, err := q.ListVulnerabilities(ctx, dupParams)
	require.NoError(t, err)
	assert.Len(t, duplicates, 1)
	assert.Equal(t, "LWL-TEST-DUP", duplicates[0].VulnerabilityID)
	require.NotNil(t, duplicates[0].DuplicateOf)
	assert.Equal(t, "LWL-TEST-CRIT-STANDARD", *duplicates[0].DuplicateOf)

	blockedParams := listParams(customerA)
	blockedParams.Flags = []string{"blocked"}
	blocked, err := q.ListVulnerabilities(ctx, blockedParams)
	require.NoError(t, err)
	assert.Equal(t, []string{"LWL-TEST-CRIT-EXTENSIVE"}, vulnIDs(blocked))

	flagOrParams := listParams(customerA)
	flagOrParams.Flags = []string{"embargo", "blocked"}
	flagOr, err := q.ListVulnerabilities(ctx, flagOrParams)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"LWL-TEST-CRIT-STANDARD", "LWL-TEST-CRIT-EXTENSIVE"}, vulnIDs(flagOr))

	for _, row := range allForA {
		if row.VulnerabilityID != "LWL-TEST-DUP" {
			assert.Nil(t, row.DuplicateOf)
		}
	}

	searchComponent := listParams(customerA)
	searchComponent.Search = textSearch("log4j")
	byComponent, err := q.ListVulnerabilities(ctx, searchComponent)
	require.NoError(t, err)
	assert.Len(t, byComponent, 1)
	assert.Equal(t, "log4j-core", byComponent[0].ComponentName)

	searchTitle := listParams(customerA)
	searchTitle.Search = textSearch("gadget")
	byTitle, err := q.ListVulnerabilities(ctx, searchTitle)
	require.NoError(t, err)
	assert.Len(t, byTitle, 1)
	assert.Equal(t, "LWL-TEST-CRIT-EXTENSIVE", byTitle[0].VulnerabilityID)

	searchID := listParams(customerA)
	searchID.Search = textSearch("LWL-TEST-MOD")
	byID, err := q.ListVulnerabilities(ctx, searchID)
	require.NoError(t, err)
	assert.Len(t, byID, 1)
	assert.Equal(t, "LWL-TEST-MOD-COMPLEX", byID[0].VulnerabilityID)

	searchCase := listParams(customerA)
	searchCase.Search = textSearch("LOG4J")
	byCase, err := q.ListVulnerabilities(ctx, searchCase)
	require.NoError(t, err)
	assert.Equal(t, []string{"LWL-TEST-CRIT-STANDARD"}, vulnIDs(byCase))
}

func TestStore_TicketsScopedToCustomer(t *testing.T) {
	ctx, tx, q := beginTestTx(t)
	defer rollbackTestTx(t, tx)

	customerA := fmt.Sprintf("lw-ticket-a-%d", time.Now().UnixNano())
	customerB := fmt.Sprintf("lw-ticket-b-%d", time.Now().UnixNano())
	insertTestVulnerabilities(t, ctx, tx, []testVulnSpec{{
		vulnID:      "LWL-TEST-SHARED-TICKETS",
		severity:    "Moderate",
		stage:       "Submitted",
		language:    "java",
		complexity:  "Standard",
		ticketIDs:   []string{"ticket-a-only"},
		daysAgo:     1,
		component:   "log4j-core",
		title:       "Shared vuln tickets",
		customerIDs: []string{customerA, customerB},
	}})

	forA, err := q.ListVulnerabilities(ctx, listParams(customerA))
	require.NoError(t, err)
	require.Len(t, forA, 1)
	assert.Equal(t, []string{"ticket-a-only"}, forA[0].LtwlsuptTicketIds)

	forB, err := q.ListVulnerabilities(ctx, listParams(customerB))
	require.NoError(t, err)
	require.Len(t, forB, 1)
	assert.Empty(t, forB[0].LtwlsuptTicketIds)

	filterB := listParams(customerB)
	filterB.LtwlsuptTicketIds = []string{"ticket-a-only"}
	filteredB, err := q.ListVulnerabilities(ctx, filterB)
	require.NoError(t, err)
	assert.Empty(t, filteredB)
}

func TestStore_CountAggregates(t *testing.T) {
	ctx, tx, q := beginTestTx(t)
	defer rollbackTestTx(t, tx)

	customerID := fmt.Sprintf("lw-agg-%d", time.Now().UnixNano())
	specs := []testVulnSpec{
		{
			vulnID:      "LWL-AGG-CRIT-STANDARD",
			severity:    "Critical",
			stage:       "Submitted",
			language:    "java",
			complexity:  "Standard",
			ticketID:    "ticket-1",
			daysAgo:     8,
			embargo:     true,
			customerIDs: []string{customerID},
		},
		{
			vulnID:      "LWL-AGG-CRIT-EXTENSIVE",
			severity:    "Critical",
			stage:       "Submitted",
			language:    "java",
			complexity:  "Extensive",
			ticketID:    "ticket-1",
			daysAgo:     31,
			customerIDs: []string{customerID},
		},
		{
			vulnID:      "LWL-AGG-MOD-COMPLEX",
			severity:    "Moderate",
			stage:       "Submitted",
			language:    "python",
			complexity:  "Complex",
			ticketID:    "ticket-1",
			daysAgo:     16,
			customerIDs: []string{customerID},
		},
		{
			vulnID:      "LWL-AGG-NOT-BLOCKED-STANDARD",
			severity:    "Moderate",
			stage:       "Submitted",
			language:    "python",
			complexity:  "Standard",
			ticketID:    "ticket-1",
			daysAgo:     5,
			customerIDs: []string{customerID},
		},
		{
			vulnID:      "LWL-AGG-NETWORK",
			severity:    "Important",
			stage:       "Lightwell Network",
			language:    "javascript",
			complexity:  "Extensive",
			ticketID:    "ticket-1",
			daysAgo:     60,
			customerIDs: []string{customerID},
		},
	}
	insertTestVulnerabilities(t, ctx, tx, specs)

	agg, err := q.CountAggregates(ctx, filterParams(customerID))
	require.NoError(t, err)
	assert.Equal(t, int64(5), agg.TotalCount)
	assert.Equal(t, int64(2), agg.CriticalCount)
	assert.Equal(t, int64(1), agg.EmbargoCount)
	assert.Equal(t, int64(1), agg.BlockedCount)

	criticalOnly := filterParams(customerID)
	criticalOnly.Severities = []string{"Critical"}
	filteredAgg, err := q.CountAggregates(ctx, criticalOnly)
	require.NoError(t, err)
	assert.Equal(t, int64(2), filteredAgg.TotalCount)
	assert.Equal(t, int64(2), filteredAgg.CriticalCount)
	assert.Equal(t, int64(1), filteredAgg.EmbargoCount)
	assert.Equal(t, int64(1), filteredAgg.BlockedCount)
}

func TestStore_CountByStage(t *testing.T) {
	ctx, tx, q := beginTestTx(t)
	defer rollbackTestTx(t, tx)

	customerID := fmt.Sprintf("lw-stage-%d", time.Now().UnixNano())
	specs := []testVulnSpec{
		{
			vulnID:      "LWL-STAGE-1",
			severity:    "Critical",
			stage:       "Submitted",
			language:    "java",
			complexity:  "Standard",
			ticketID:    "ticket-1",
			daysAgo:     3,
			customerIDs: []string{customerID},
		},
		{
			vulnID:      "LWL-STAGE-2",
			severity:    "Important",
			stage:       "Submitted",
			language:    "java",
			complexity:  "Complex",
			ticketID:    "ticket-1",
			daysAgo:     4,
			customerIDs: []string{customerID},
		},
		{
			vulnID:      "LWL-STAGE-3",
			severity:    "Moderate",
			stage:       "Classified",
			language:    "python",
			complexity:  "Standard",
			ticketID:    "ticket-1",
			daysAgo:     5,
			customerIDs: []string{customerID},
		},
	}
	insertTestVulnerabilities(t, ctx, tx, specs)

	rows, err := q.CountByStage(ctx, stageParams(customerID))
	require.NoError(t, err)
	assert.Len(t, rows, 2)

	counts := map[string]int64{}
	for _, row := range rows {
		counts[row.Stage] = row.Count
	}
	assert.Equal(t, int64(2), counts["Submitted"])
	assert.Equal(t, int64(1), counts["Classified"])

	classifiedOnly := stageParams(customerID)
	classifiedOnly.Stages = []string{"Classified"}
	filteredStages, err := q.CountByStage(ctx, classifiedOnly)
	require.NoError(t, err)
	require.Len(t, filteredStages, 1)
	assert.Equal(t, "Classified", filteredStages[0].Stage)
	assert.Equal(t, int64(1), filteredStages[0].Count)
}

func TestStore_EmptyResultsForUnknownFilters(t *testing.T) {
	ctx, tx, q := beginTestTx(t)
	defer rollbackTestTx(t, tx)

	customerID := fmt.Sprintf("lw-empty-%d", time.Now().UnixNano())
	insertTestVulnerabilities(t, ctx, tx, []testVulnSpec{
		{
			vulnID:      "LWL-EMPTY-1",
			severity:    "Moderate",
			stage:       "Submitted",
			language:    "java",
			complexity:  "Standard",
			ticketID:    "ticket-1",
			daysAgo:     2,
			customerIDs: []string{customerID},
		},
	})

	emptySlice := listParams(customerID)
	emptySlice.Severities = []string{}
	emptySlice.Stages = []string{}
	emptySlice.Complexities = []string{}
	emptySlice.LtwlsuptTicketIds = []string{}
	unfiltered, err := q.ListVulnerabilities(ctx, emptySlice)
	require.NoError(t, err)
	assert.Equal(t, []string{"LWL-EMPTY-1"}, vulnIDs(unfiltered))

	cases := []struct {
		name   string
		mutate func(*store.ListVulnerabilitiesParams)
	}{
		{"severity", func(p *store.ListVulnerabilitiesParams) { p.Severities = []string{"NonexistentSeverity"} }},
		{"stage", func(p *store.ListVulnerabilitiesParams) { p.Stages = []string{"NonexistentStage"} }},
		{"complexity", func(p *store.ListVulnerabilitiesParams) { p.Complexities = []string{"NonexistentComplexity"} }},
		{"ticket", func(p *store.ListVulnerabilitiesParams) { p.LtwlsuptTicketIds = []string{"no-such-ticket"} }},
		{"flag", func(p *store.ListVulnerabilitiesParams) { p.Flags = []string{"bogus"} }},
		{"search", func(p *store.ListVulnerabilitiesParams) { p.Search = textSearch("zzzz-no-match") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			list := listParams(customerID)
			tc.mutate(&list)
			items, err := q.ListVulnerabilities(ctx, list)
			require.NoError(t, err)
			assert.Empty(t, items)

			aggParams := filterParams(customerID)
			aggParams.Severities = list.Severities
			aggParams.Stages = list.Stages
			aggParams.Complexities = list.Complexities
			aggParams.LtwlsuptTicketIds = list.LtwlsuptTicketIds
			aggParams.Flags = list.Flags
			aggParams.Search = list.Search
			agg, err := q.CountAggregates(ctx, aggParams)
			require.NoError(t, err)
			assert.Equal(t, int64(0), agg.TotalCount)

			stageFilter := stageParams(customerID)
			stageFilter.Severities = list.Severities
			stageFilter.Stages = list.Stages
			stageFilter.Complexities = list.Complexities
			stageFilter.LtwlsuptTicketIds = list.LtwlsuptTicketIds
			stageFilter.Flags = list.Flags
			stageFilter.Search = list.Search
			stageRows, err := q.CountByStage(ctx, stageFilter)
			require.NoError(t, err)
			assert.Empty(t, stageRows)
		})
	}
}

func TestStore_MultiValueAndCombinedFilters(t *testing.T) {
	ctx, tx, q := beginTestTx(t)
	defer rollbackTestTx(t, tx)

	customerID := fmt.Sprintf("lw-multi-%d", time.Now().UnixNano())
	insertTestVulnerabilities(t, ctx, tx, []testVulnSpec{
		{
			vulnID:      "LWL-MULTI-CRIT-STD",
			severity:    "Critical",
			stage:       "Submitted",
			language:    "java",
			complexity:  "Standard",
			ticketID:    "ticket-a",
			daysAgo:     2,
			customerIDs: []string{customerID},
		},
		{
			vulnID:      "LWL-MULTI-MOD-STD",
			severity:    "Moderate",
			stage:       "Submitted",
			language:    "java",
			complexity:  "Standard",
			ticketID:    "ticket-a",
			daysAgo:     3,
			customerIDs: []string{customerID},
		},
		{
			vulnID:      "LWL-MULTI-CRIT-CPLX",
			severity:    "Critical",
			stage:       "Classified",
			language:    "python",
			complexity:  "Complex",
			ticketID:    "ticket-b",
			daysAgo:     4,
			customerIDs: []string{customerID},
		},
		{
			vulnID:      "LWL-MULTI-IMP-STD",
			severity:    "Important",
			stage:       "Submitted",
			language:    "java",
			complexity:  "Standard",
			ticketID:    "ticket-a",
			daysAgo:     5,
			customerIDs: []string{customerID},
		},
	})

	orSeverity := listParams(customerID)
	orSeverity.Severities = []string{"Critical", "Moderate"}
	orRows, err := q.ListVulnerabilities(ctx, orSeverity)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"LWL-MULTI-CRIT-STD", "LWL-MULTI-MOD-STD", "LWL-MULTI-CRIT-CPLX"}, vulnIDs(orRows))

	orStage := listParams(customerID)
	orStage.Stages = []string{"Submitted", "Classified"}
	orStageRows, err := q.ListVulnerabilities(ctx, orStage)
	require.NoError(t, err)
	assert.Len(t, orStageRows, 4)

	andFilters := listParams(customerID)
	andFilters.Severities = []string{"Critical"}
	andFilters.Stages = []string{"Submitted"}
	andFilters.Complexities = []string{"Standard"}
	andFilters.LtwlsuptTicketIds = []string{"ticket-a"}
	andRows, err := q.ListVulnerabilities(ctx, andFilters)
	require.NoError(t, err)
	assert.Equal(t, []string{"LWL-MULTI-CRIT-STD"}, vulnIDs(andRows))
}

func TestStore_Pagination(t *testing.T) {
	ctx, tx, q := beginTestTx(t)
	defer rollbackTestTx(t, tx)

	customerID := fmt.Sprintf("lw-page-%d", time.Now().UnixNano())
	insertTestVulnerabilities(t, ctx, tx, []testVulnSpec{
		{vulnID: "LWL-PAGE-A", severity: "Moderate", stage: "Submitted", language: "java", complexity: "Standard", ticketID: "ticket-1", daysAgo: 1, customerIDs: []string{customerID}},
		{vulnID: "LWL-PAGE-B", severity: "Moderate", stage: "Submitted", language: "java", complexity: "Standard", ticketID: "ticket-1", daysAgo: 1, customerIDs: []string{customerID}},
		{vulnID: "LWL-PAGE-C", severity: "Moderate", stage: "Submitted", language: "java", complexity: "Standard", ticketID: "ticket-1", daysAgo: 1, customerIDs: []string{customerID}},
	})

	for i, vulnID := range []string{"LWL-PAGE-A", "LWL-PAGE-B", "LWL-PAGE-C"} {
		_, err := tx.Exec(ctx, `
			UPDATE lightwell_vulnerabilities
			SET last_updated = TIMESTAMPTZ '2026-08-18 12:00:00+00' + ($1 * INTERVAL '1 hour')
			WHERE vulnerability_id = $2`,
			i, vulnID,
		)
		require.NoError(t, err)
	}

	page1 := listParams(customerID)
	page1.PageLimit = 2
	page1.PageOffset = 0
	firstPage, err := q.ListVulnerabilities(ctx, page1)
	require.NoError(t, err)
	assert.Equal(t, []string{"LWL-PAGE-C", "LWL-PAGE-B"}, vulnIDs(firstPage))

	page2 := listParams(customerID)
	page2.PageLimit = 2
	page2.PageOffset = 2
	secondPage, err := q.ListVulnerabilities(ctx, page2)
	require.NoError(t, err)
	assert.Equal(t, []string{"LWL-PAGE-A"}, vulnIDs(secondPage))
}

func TestStore_ListCustomerIds(t *testing.T) {
	ctx, tx, q := beginTestTx(t)
	defer rollbackTestTx(t, tx)

	customerA := fmt.Sprintf("lw-list-a-%d", time.Now().UnixNano())
	customerB := fmt.Sprintf("lw-list-b-%d", time.Now().UnixNano())
	insertTestVulnerabilities(t, ctx, tx, []testVulnSpec{
		{
			vulnID:      "LWL-LIST-1",
			severity:    "Moderate",
			stage:       "Submitted",
			language:    "java",
			complexity:  "Standard",
			ticketID:    "ticket-1",
			daysAgo:     1,
			customerIDs: []string{customerA},
		},
		{
			vulnID:      "LWL-LIST-2",
			severity:    "Moderate",
			stage:       "Submitted",
			language:    "python",
			complexity:  "Standard",
			ticketID:    "ticket-1",
			daysAgo:     1,
			customerIDs: []string{customerB},
		},
	})

	ids, err := q.ListCustomerIds(ctx)
	require.NoError(t, err)
	assert.Contains(t, ids, customerA)
	assert.Contains(t, ids, customerB)
}

func TestStore_ListLtwlsuptTicketIds(t *testing.T) {
	ctx, tx, q := beginTestTx(t)
	defer rollbackTestTx(t, tx)

	customerA := fmt.Sprintf("lw-tickets-a-%d", time.Now().UnixNano())
	customerB := fmt.Sprintf("lw-tickets-b-%d", time.Now().UnixNano())
	insertTestVulnerabilities(t, ctx, tx, []testVulnSpec{
		{
			vulnID:      "LWL-TICKETS-1",
			severity:    "Moderate",
			stage:       "Submitted",
			language:    "java",
			complexity:  "Standard",
			ticketIDs:   []string{"ticket-c", "ticket-a"},
			daysAgo:     1,
			customerIDs: []string{customerA},
		},
		{
			vulnID:      "LWL-TICKETS-2",
			severity:    "Low",
			stage:       "Submitted",
			language:    "java",
			complexity:  "Standard",
			ticketID:    "ticket-a",
			daysAgo:     1,
			customerIDs: []string{customerA},
		},
		{
			vulnID:      "LWL-TICKETS-3",
			severity:    "Low",
			stage:       "Submitted",
			language:    "python",
			complexity:  "Standard",
			ticketID:    "ticket-b",
			daysAgo:     1,
			customerIDs: []string{customerB},
		},
	})

	ids, err := q.ListLtwlsuptTicketIds(ctx, customerA)
	require.NoError(t, err)
	assert.Equal(t, []string{"ticket-a", "ticket-c"}, ids)

	ids, err = q.ListLtwlsuptTicketIds(ctx, customerB)
	require.NoError(t, err)
	assert.Equal(t, []string{"ticket-b"}, ids)

	ids, err = q.ListLtwlsuptTicketIds(ctx, "no-such-customer")
	require.NoError(t, err)
	assert.Empty(t, ids)
}

func TestStore_UpsertAllowsDuplicateVulnerabilityIDs(t *testing.T) {
	ctx, tx, q := beginTestTx(t)
	defer rollbackTestTx(t, tx)

	sharedID := fmt.Sprintf("LW-%d", time.Now().UnixNano())
	first := store.UpsertVulnerabilityParams{
		Uuid:             uuid.New(),
		VulnerabilityKey: fmt.Sprintf("LTWL-%d-A", time.Now().UnixNano()),
		VulnerabilityID:  sharedID,
		ComponentName:    "component",
		ComponentVersion: "1.0",
		Severity:         "Important",
		Stage:            "Submitted",
		Complexity:       "",
		SubmittedDate:    time.Now().UTC(),
		LastUpdated:      time.Now().UTC().Truncate(time.Second),
	}
	second := first
	second.Uuid = uuid.New()
	second.VulnerabilityKey = fmt.Sprintf("LTWL-%d-B", time.Now().UnixNano())

	insertedFirst, err := q.UpsertVulnerability(ctx, first)
	require.NoError(t, err)
	insertedSecond, err := q.UpsertVulnerability(ctx, second)
	require.NoError(t, err)
	assert.NotEqual(t, insertedFirst.Uuid, insertedSecond.Uuid)

	gotFirst, err := q.GetVulnerabilityByKey(ctx, first.VulnerabilityKey)
	require.NoError(t, err)
	gotSecond, err := q.GetVulnerabilityByKey(ctx, second.VulnerabilityKey)
	require.NoError(t, err)
	assert.Equal(t, sharedID, gotFirst.VulnerabilityID)
	assert.Equal(t, sharedID, gotSecond.VulnerabilityID)
}

func TestStore_UpsertVulnerabilityIsIdempotent(t *testing.T) {
	ctx, tx, q := beginTestTx(t)
	defer rollbackTestTx(t, tx)

	params := store.UpsertVulnerabilityParams{
		Uuid:               uuid.New(),
		VulnerabilityKey:   fmt.Sprintf("LTWL-%d-A", time.Now().UnixNano()),
		VulnerabilityID:    fmt.Sprintf("LWL-UPSERT-%d", time.Now().UnixNano()),
		ComponentName:      "component",
		ComponentVersion:   "1.0",
		Severity:           "Important",
		ExploitTested:      false,
		ReproducerIncluded: false,
		Stage:              "Submitted",
		Complexity:         "",
		SubmittedDate:      time.Now().UTC(),
		LastUpdated:        time.Now().UTC().Truncate(time.Second),
		Embargo:            false,
		Duplicate:          false,
	}

	inserted, err := q.UpsertVulnerability(ctx, params)
	require.NoError(t, err)
	assert.True(t, inserted.Inserted)

	before, err := q.GetVulnerabilityByKey(ctx, params.VulnerabilityKey)
	require.NoError(t, err)
	_, err = q.UpsertVulnerability(ctx, params)
	require.ErrorIs(t, err, pgx.ErrNoRows)
	after, err := q.GetVulnerabilityByKey(ctx, params.VulnerabilityKey)
	require.NoError(t, err)
	assert.Equal(t, before.UpdatedAt, after.UpdatedAt)

	params.Stage = "Classified"
	updated, err := q.UpsertVulnerability(ctx, params)
	require.NoError(t, err)
	assert.False(t, updated.Inserted)
	after, err = q.GetVulnerabilityByKey(ctx, params.VulnerabilityKey)
	require.NoError(t, err)
	assert.Equal(t, "Classified", after.Stage)
	assert.Equal(t, inserted.Uuid, after.Uuid)

	customer := store.InsertVulnerabilityCustomerParams{CustomerID: "customer-1", VulnerabilityUuid: inserted.Uuid}
	require.NoError(t, q.InsertVulnerabilityCustomer(ctx, customer))
	require.NoError(t, q.InsertVulnerabilityCustomer(ctx, customer))
	var count int
	require.NoError(t, tx.QueryRow(ctx,
		`SELECT COUNT(*) FROM lightwell_vulnerability_customers WHERE customer_id = $1 AND vulnerability_uuid = $2`,
		customer.CustomerID, customer.VulnerabilityUuid,
	).Scan(&count))
	assert.Equal(t, 1, count)

	ticket := store.UpsertVulnerabilityTicketParams{VulnerabilityUuid: inserted.Uuid, CustomerID: "customer-1", TicketID: "EPIC-1"}
	require.NoError(t, q.UpsertVulnerabilityTicket(ctx, ticket))
	require.NoError(t, q.UpsertVulnerabilityTicket(ctx, ticket))
	var ticketCount int
	require.NoError(t, tx.QueryRow(ctx,
		`SELECT COUNT(*) FROM lightwell_vulnerability_support_tickets WHERE vulnerability_uuid = $1 AND ticket_id = $2`,
		ticket.VulnerabilityUuid, ticket.TicketID,
	).Scan(&ticketCount))
	assert.Equal(t, 1, ticketCount)
}
