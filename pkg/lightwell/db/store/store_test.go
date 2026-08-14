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
	vulnID            string
	severity          string
	stage             string
	language          string
	complexity        string
	ltwwlsuptTicketID string
	daysAgo           int
	embargo           bool
	duplicate         bool
	component         string
	title             string
	customerIDs       []string
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
				uuid, vulnerability_id, component_name, component_version, title, severity, stage, language, complexity,
				submitted_date, last_updated, embargo, duplicate, ltwwlsupt_ticket_id
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7, $8, $9, CURRENT_DATE - $10::int, NOW(), $11, $12, $13
			)`,
			id,
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
			spec.ltwwlsuptTicketID,
		)
		require.NoError(t, err)

		for _, customerID := range spec.customerIDs {
			_, err = tx.Exec(ctx,
				`INSERT INTO lightwell_vulnerability_customers (customer_id, vulnerability_uuid) VALUES ($1, $2)`,
				customerID, id,
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

func TestStore_CustomerScopingAndFilters(t *testing.T) {
	ctx, tx, q := beginTestTx(t)
	defer rollbackTestTx(t, tx)

	customerA := fmt.Sprintf("lw-scope-a-%d", time.Now().UnixNano())
	customerB := fmt.Sprintf("lw-scope-b-%d", time.Now().UnixNano())

	specs := []testVulnSpec{
		{
			vulnID:            "LWL-TEST-CRIT-STANDARD",
			severity:          "Critical",
			stage:             "Submitted",
			language:          "java",
			complexity:        "Standard",
			ltwwlsuptTicketID: "ticket-alpha",
			daysAgo:           8,
			embargo:           true,
			component:         "log4j-core",
			title:             "JNDI injection test",
			customerIDs:       []string{customerA},
		},
		{
			vulnID:            "LWL-TEST-IMP-COMPLEX",
			severity:          "Important",
			stage:             "Submitted",
			language:          "java",
			complexity:        "Complex",
			ltwwlsuptTicketID: "ticket-alpha",
			daysAgo:           10,
			component:         "spring-web",
			title:             "HTTP smuggling test",
			customerIDs:       []string{customerA},
		},
		{
			vulnID:            "LWL-TEST-MOD-COMPLEX",
			severity:          "Moderate",
			stage:             "Classified",
			language:          "python",
			complexity:        "Complex",
			ltwwlsuptTicketID: "ticket-beta",
			daysAgo:           16,
			component:         "requests",
			title:             "SSRF redirect test",
			customerIDs:       []string{customerA},
		},
		{
			vulnID:            "LWL-TEST-CRIT-EXTENSIVE",
			severity:          "Critical",
			stage:             "Fix in Progress",
			language:          "java",
			complexity:        "Extensive",
			ltwwlsuptTicketID: "ticket-beta",
			daysAgo:           31,
			component:         "jackson-databind",
			title:             "Deserialization gadget test",
			customerIDs:       []string{customerA},
		},
		{
			vulnID:            "LWL-TEST-NETWORK",
			severity:          "Important",
			stage:             "Lightwell Network",
			language:          "javascript",
			complexity:        "Extensive",
			ltwwlsuptTicketID: "ticket-alpha",
			daysAgo:           50,
			component:         "express",
			title:             "Path traversal test",
			customerIDs:       []string{customerA},
		},
		{
			vulnID:            "LWL-TEST-DUP",
			severity:          "Moderate",
			stage:             "Validation",
			language:          "javascript",
			complexity:        "Standard",
			ltwwlsuptTicketID: "ticket-beta",
			daysAgo:           20,
			duplicate:         true,
			component:         "lodash",
			title:             "Prototype pollution test",
			customerIDs:       []string{customerA},
		},
		{
			vulnID:            "LWL-TEST-OTHER-CUSTOMER",
			severity:          "Critical",
			stage:             "Submitted",
			language:          "java",
			complexity:        "Standard",
			ltwwlsuptTicketID: "ticket-other",
			daysAgo:           9,
			component:         "other-package",
			title:             "Other customer only",
			customerIDs:       []string{customerB},
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
	ticketParams.LtwwlsuptTicketIds = []string{"ticket-beta"}
	beta, err := q.ListVulnerabilities(ctx, ticketParams)
	require.NoError(t, err)
	assert.Len(t, beta, 3)

	embargoParams := listParams(customerA)
	embargoParams.Flag = textFlag("embargo")
	embargoed, err := q.ListVulnerabilities(ctx, embargoParams)
	require.NoError(t, err)
	assert.Len(t, embargoed, 1)
	assert.Equal(t, "LWL-TEST-CRIT-STANDARD", embargoed[0].VulnerabilityID)

	dupParams := listParams(customerA)
	dupParams.Flag = textFlag("duplicate")
	duplicates, err := q.ListVulnerabilities(ctx, dupParams)
	require.NoError(t, err)
	assert.Len(t, duplicates, 1)
	assert.Equal(t, "LWL-TEST-DUP", duplicates[0].VulnerabilityID)

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
}

func TestStore_CountAggregates(t *testing.T) {
	ctx, tx, q := beginTestTx(t)
	defer rollbackTestTx(t, tx)

	customerID := fmt.Sprintf("lw-agg-%d", time.Now().UnixNano())
	specs := []testVulnSpec{
		{
			vulnID:            "LWL-AGG-CRIT-STANDARD",
			severity:          "Critical",
			stage:             "Submitted",
			language:          "java",
			complexity:        "Standard",
			ltwwlsuptTicketID: "ticket-1",
			daysAgo:           8,
			embargo:           true,
			customerIDs:       []string{customerID},
		},
		{
			vulnID:            "LWL-AGG-CRIT-EXTENSIVE",
			severity:          "Critical",
			stage:             "Submitted",
			language:          "java",
			complexity:        "Extensive",
			ltwwlsuptTicketID: "ticket-1",
			daysAgo:           31,
			customerIDs:       []string{customerID},
		},
		{
			vulnID:            "LWL-AGG-MOD-COMPLEX",
			severity:          "Moderate",
			stage:             "Submitted",
			language:          "python",
			complexity:        "Complex",
			ltwwlsuptTicketID: "ticket-1",
			daysAgo:           16,
			customerIDs:       []string{customerID},
		},
		{
			vulnID:            "LWL-AGG-NOT-BLOCKED-STANDARD",
			severity:          "Moderate",
			stage:             "Submitted",
			language:          "python",
			complexity:        "Standard",
			ltwwlsuptTicketID: "ticket-1",
			daysAgo:           5,
			customerIDs:       []string{customerID},
		},
		{
			vulnID:            "LWL-AGG-NETWORK",
			severity:          "Important",
			stage:             "Lightwell Network",
			language:          "javascript",
			complexity:        "Extensive",
			ltwwlsuptTicketID: "ticket-1",
			daysAgo:           60,
			customerIDs:       []string{customerID},
		},
	}
	insertTestVulnerabilities(t, ctx, tx, specs)

	agg, err := q.CountAggregates(ctx, filterParams(customerID))
	require.NoError(t, err)
	assert.Equal(t, int64(5), agg.TotalCount)
	assert.Equal(t, int64(2), agg.CriticalCount)
	assert.Equal(t, int64(1), agg.EmbargoCount)
	assert.Equal(t, int64(1), agg.BlockedCount)
}

func TestStore_CountByStage(t *testing.T) {
	ctx, tx, q := beginTestTx(t)
	defer rollbackTestTx(t, tx)

	customerID := fmt.Sprintf("lw-stage-%d", time.Now().UnixNano())
	specs := []testVulnSpec{
		{
			vulnID:            "LWL-STAGE-1",
			severity:          "Critical",
			stage:             "Submitted",
			language:          "java",
			complexity:        "Standard",
			ltwwlsuptTicketID: "ticket-1",
			daysAgo:           3,
			customerIDs:       []string{customerID},
		},
		{
			vulnID:            "LWL-STAGE-2",
			severity:          "Important",
			stage:             "Submitted",
			language:          "java",
			complexity:        "Complex",
			ltwwlsuptTicketID: "ticket-1",
			daysAgo:           4,
			customerIDs:       []string{customerID},
		},
		{
			vulnID:            "LWL-STAGE-3",
			severity:          "Moderate",
			stage:             "Classified",
			language:          "python",
			complexity:        "Standard",
			ltwwlsuptTicketID: "ticket-1",
			daysAgo:           5,
			customerIDs:       []string{customerID},
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
}

func TestStore_EmptyResultsForUnknownFilters(t *testing.T) {
	ctx, tx, q := beginTestTx(t)
	defer rollbackTestTx(t, tx)

	customerID := fmt.Sprintf("lw-empty-%d", time.Now().UnixNano())
	insertTestVulnerabilities(t, ctx, tx, []testVulnSpec{
		{
			vulnID:            "LWL-EMPTY-1",
			severity:          "Moderate",
			stage:             "Submitted",
			language:          "java",
			complexity:        "Standard",
			ltwwlsuptTicketID: "ticket-1",
			daysAgo:           2,
			customerIDs:       []string{customerID},
		},
	})

	params := filterParams(customerID)
	params.Severities = []string{"NonexistentSeverity"}
	agg, err := q.CountAggregates(ctx, params)
	require.NoError(t, err)
	assert.Equal(t, int64(0), agg.TotalCount)
	assert.Equal(t, int64(0), agg.CriticalCount)
	assert.Equal(t, int64(0), agg.EmbargoCount)
	assert.Equal(t, int64(0), agg.BlockedCount)

	list := listParams(customerID)
	list.Severities = []string{"NonexistentSeverity"}
	items, err := q.ListVulnerabilities(ctx, list)
	require.NoError(t, err)
	assert.Empty(t, items)

	stageParamsUnknown := stageParams(customerID)
	stageParamsUnknown.Severities = []string{"NonexistentSeverity"}
	stageRowsUnknown, err := q.CountByStage(ctx, stageParamsUnknown)
	require.NoError(t, err)
	assert.Empty(t, stageRowsUnknown)
}

func TestStore_ListCustomerIds(t *testing.T) {
	ctx, tx, q := beginTestTx(t)
	defer rollbackTestTx(t, tx)

	customerA := fmt.Sprintf("lw-list-a-%d", time.Now().UnixNano())
	customerB := fmt.Sprintf("lw-list-b-%d", time.Now().UnixNano())
	insertTestVulnerabilities(t, ctx, tx, []testVulnSpec{
		{
			vulnID:            "LWL-LIST-1",
			severity:          "Moderate",
			stage:             "Submitted",
			language:          "java",
			complexity:        "Standard",
			ltwwlsuptTicketID: "ticket-1",
			daysAgo:           1,
			customerIDs:       []string{customerA},
		},
		{
			vulnID:            "LWL-LIST-2",
			severity:          "Moderate",
			stage:             "Submitted",
			language:          "python",
			complexity:        "Standard",
			ltwwlsuptTicketID: "ticket-1",
			daysAgo:           1,
			customerIDs:       []string{customerB},
		},
	})

	ids, err := q.ListCustomerIds(ctx)
	require.NoError(t, err)
	assert.Contains(t, ids, customerA)
	assert.Contains(t, ids, customerB)
}
