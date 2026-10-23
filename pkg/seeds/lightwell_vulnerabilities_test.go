package seeds

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	deleteLightwellDemoSeedMigration = "db/migrations/20261023215619_delete_lightwell_demo_seed.up.sql"
	lightwellSeedVulnerabilityCount  = 73
)

var lightwellSeedBeaconProcessedAt = time.Date(2026, time.August, 18, 7, 0, 0, 0, time.UTC)

func TestStatementsFromSQL(t *testing.T) {
	sqlText := `-- comment
BEGIN;
INSERT INTO lightwell_vulnerabilities (uuid) VALUES ('00000000-0000-4000-8000-000000000001');
COMMIT;
`
	assert.Equal(t, []string{
		"INSERT INTO lightwell_vulnerabilities (uuid) VALUES ('00000000-0000-4000-8000-000000000001')",
	}, statementsFromSQL(sqlText))
}

func TestLightwellSeedIncludesBeaconTimestamp(t *testing.T) {
	sqlText, err := readRepoFile(lightwellSeedSQLRelPath)
	require.NoError(t, err)
	assert.Contains(t, sqlText, "INSERT INTO lightwell_beacon_sync")
	assert.Contains(t, sqlText, "ON CONFLICT (id) DO NOTHING")
	assert.Contains(t, sqlText, "2026-08-18 07:00:00+00")

	var beaconStmt string
	for _, stmt := range statementsFromSQL(sqlText) {
		if strings.Contains(stmt, "lightwell_beacon_sync") {
			beaconStmt = stmt
		}
	}
	assert.Contains(t, beaconStmt, "ON CONFLICT (id) DO NOTHING")
}

func TestDemoSeedDeletionIsScoped(t *testing.T) {
	sqlText, err := readRepoFile(deleteLightwellDemoSeedMigration)
	require.NoError(t, err)

	stmts := statementsFromSQL(sqlText)
	require.Len(t, stmts, 3)
	for _, stmt := range stmts {
		assert.Contains(t, stmt, "uuid::text LIKE '00000000-0000-4000-8000-%'")
		assert.Contains(t, stmt, "vulnerability_key LIKE 'PROJ-%'")
		assert.NotContains(t, strings.ToLower(stmt), "lightwell_beacon_sync")
		assert.NotContains(t, stmt, "demo-customer")
	}
}

func (s *SeedSuite) TestSeedLightwellVulnerabilities() {
	t := s.T()
	require.NotNil(t, s.tx)

	s.deleteLightwellSeedRows()
	s.Require().NoError(s.tx.Exec(`DELETE FROM lightwell_beacon_sync`).Error)

	err := SeedLightwellVulnerabilities(s.tx)
	require.NoError(t, err)

	var vulnCount int64
	err = s.tx.Raw("SELECT COUNT(*) FROM lightwell_vulnerabilities WHERE uuid = ?", lightwellSeedSentinelUUID).Scan(&vulnCount).Error
	require.NoError(t, err)
	assert.Equal(t, int64(1), vulnCount)

	var total int64
	err = s.tx.Raw("SELECT COUNT(*) FROM lightwell_vulnerabilities WHERE uuid::text LIKE '00000000-0000-4000-8000-%'").Scan(&total).Error
	require.NoError(t, err)
	assert.Equal(t, int64(lightwellSeedVulnerabilityCount), total)
	assert.Equal(t, int64(4), s.count(`SELECT COUNT(*) FROM lightwell_vulnerabilities WHERE uuid::text LIKE '00000000-0000-4000-8000-%' AND stage = 'On Hold'`))

	var reasons []string
	err = s.tx.Raw(`
		SELECT resolution_reason
		FROM lightwell_vulnerabilities
		WHERE uuid::text LIKE '00000000-0000-4000-8000-%' AND stage = 'Unremediated'
		ORDER BY resolution_reason COLLATE "C"
	`).Scan(&reasons).Error
	require.NoError(t, err)
	assert.Equal(t, []string{
		"Commercial Package",
		"Component not Present",
		"Fix Exists in Lightwell Remediated Package",
		"Fixed in Upstream",
		"Inline mitigations already exist",
		"Invalid License",
		"Not a GA Version",
		"Red Hat Package",
		"Unsupported Language Ecosystem",
		"Unsupported Package Format",
		"Vulnerable code cannot be controlled by adversary",
		"Vulnerable code not in execute path",
		"Vulnerable code not present",
	}, reasons)

	var tickets []string
	err = s.tx.Raw(`
		SELECT DISTINCT ticket_id
		FROM lightwell_vulnerability_support_tickets
		WHERE vulnerability_uuid::text LIKE '00000000-0000-4000-8000-%'
		ORDER BY ticket_id
	`).Scan(&tickets).Error
	require.NoError(t, err)
	assert.Equal(t, []string{"demo-tk-1", "demo-tk-2", "demo-tk-3"}, tickets)
	assert.True(t, lightwellSeedBeaconProcessedAt.Equal(s.beaconProcessedAt()))

	err = SeedLightwellVulnerabilities(s.tx)
	require.NoError(t, err)
	assert.True(t, lightwellSeedBeaconProcessedAt.Equal(s.beaconProcessedAt()))

	syncedAt := time.Date(2026, time.October, 1, 12, 0, 0, 0, time.UTC)
	s.Require().NoError(s.tx.Exec(`UPDATE lightwell_beacon_sync SET last_processed_at = ? WHERE id = true`, syncedAt).Error)
	s.deleteLightwellSeedRows()

	err = SeedLightwellVulnerabilities(s.tx)
	require.NoError(t, err)
	assert.True(t, syncedAt.Equal(s.beaconProcessedAt()))
	assert.Equal(t, int64(lightwellSeedVulnerabilityCount), s.count(`SELECT COUNT(*) FROM lightwell_vulnerabilities WHERE uuid::text LIKE '00000000-0000-4000-8000-%'`))
}

func (s *SeedSuite) TestDemoSeedDeletionLeavesSyncedRows() {
	t := s.T()
	require.NotNil(t, s.tx)

	s.deleteLightwellSeedRows()
	s.Require().NoError(s.tx.Exec(`DELETE FROM lightwell_beacon_sync`).Error)
	s.Require().NoError(SeedLightwellVulnerabilities(s.tx))

	syncedAt := time.Date(2026, time.October, 1, 15, 0, 0, 0, time.UTC)
	s.Require().NoError(s.tx.Exec(`UPDATE lightwell_beacon_sync SET last_processed_at = ? WHERE id = true`, syncedAt).Error)

	prefixOnly := uuid.MustParse("00000000-0000-4000-8000-0000000000ff")
	projKeyOnly := uuid.New()
	synced := uuid.New()
	s.insertSyncedVulnerability(prefixOnly, "LTWL-PREFIX-ONLY", "LWL-PREFIX-ONLY")
	s.insertSyncedVulnerability(projKeyOnly, "PROJ-NOT-A-SEED-ROW", "LWL-NOT-A-SEED-ROW")
	s.insertSyncedVulnerability(synced, "LTWL-9001", "LWL-9001")

	s.applyRepoSQL(deleteLightwellDemoSeedMigration)

	assert.True(t, syncedAt.Equal(s.beaconProcessedAt()))
	assert.Equal(t, int64(0), s.count(`
		SELECT COUNT(*)
		FROM lightwell_vulnerabilities
		WHERE uuid::text LIKE '00000000-0000-4000-8000-%'
			AND vulnerability_key LIKE 'PROJ-%'
	`))
	assert.Equal(t, int64(0), s.count(`
		SELECT COUNT(*)
		FROM lightwell_vulnerability_customers AS customer
		JOIN lightwell_vulnerabilities AS vulnerability ON vulnerability.uuid = customer.vulnerability_uuid
		WHERE vulnerability.uuid::text LIKE '00000000-0000-4000-8000-%'
			AND vulnerability.vulnerability_key LIKE 'PROJ-%'
	`))
	assert.Equal(t, int64(0), s.count(`
		SELECT COUNT(*)
		FROM lightwell_vulnerability_support_tickets AS ticket
		JOIN lightwell_vulnerabilities AS vulnerability ON vulnerability.uuid = ticket.vulnerability_uuid
		WHERE vulnerability.uuid::text LIKE '00000000-0000-4000-8000-%'
			AND vulnerability.vulnerability_key LIKE 'PROJ-%'
	`))

	var kept []string
	err := s.tx.Raw(`
		SELECT vulnerability_key
		FROM lightwell_vulnerabilities
		WHERE uuid IN (?, ?, ?)
		ORDER BY vulnerability_key
	`, prefixOnly, projKeyOnly, synced).Scan(&kept).Error
	require.NoError(t, err)
	assert.Equal(t, []string{"LTWL-9001", "LTWL-PREFIX-ONLY", "PROJ-NOT-A-SEED-ROW"}, kept)
	assert.Equal(t, int64(3), s.count(`
		SELECT COUNT(*) FROM lightwell_vulnerability_customers WHERE vulnerability_uuid IN (?, ?, ?)
	`, prefixOnly, projKeyOnly, synced))
	assert.Equal(t, int64(3), s.count(`
		SELECT COUNT(*) FROM lightwell_vulnerability_support_tickets WHERE vulnerability_uuid IN (?, ?, ?)
	`, prefixOnly, projKeyOnly, synced))
}

func (s *SeedSuite) applyRepoSQL(relPath string) {
	sqlText, err := readRepoFile(relPath)
	s.Require().NoError(err)
	for _, stmt := range statementsFromSQL(sqlText) {
		s.Require().NoError(s.tx.Exec(stmt).Error)
	}
}

func (s *SeedSuite) insertSyncedVulnerability(id uuid.UUID, key string, vulnerabilityID string) {
	s.Require().NoError(s.tx.Exec(`
		INSERT INTO lightwell_vulnerabilities (
			uuid, vulnerability_key, vulnerability_id, component_name, component_version,
			severity, stage, complexity, submitted_date, last_updated
		) VALUES (?, ?, ?, 'component', '1.0.0', 'Moderate', 'Submitted', 'Standard', DATE '2026-08-01', TIMESTAMPTZ '2026-08-01 00:00:00+00')
	`, id, key, vulnerabilityID).Error)
	s.Require().NoError(s.tx.Exec(`
		INSERT INTO lightwell_vulnerability_customers (customer_id, vulnerability_uuid)
		VALUES ('synced-customer', ?)
	`, id).Error)
	s.Require().NoError(s.tx.Exec(`
		INSERT INTO lightwell_vulnerability_support_tickets (vulnerability_uuid, customer_id, ticket_id)
		VALUES (?, 'synced-customer', 'synced-ticket')
	`, id).Error)
}

func (s *SeedSuite) beaconProcessedAt() time.Time {
	var processedAt time.Time
	s.Require().NoError(s.tx.Raw(`SELECT last_processed_at FROM lightwell_beacon_sync WHERE id = true`).Scan(&processedAt).Error)
	return processedAt
}

func (s *SeedSuite) count(query string, args ...any) int64 {
	var n int64
	s.Require().NoError(s.tx.Raw(query, args...).Scan(&n).Error)
	return n
}

func (s *SeedSuite) deleteLightwellSeedRows() {
	s.Require().NoError(s.tx.Exec(`
		DELETE FROM lightwell_vulnerability_support_tickets
		WHERE vulnerability_uuid::text LIKE '00000000-0000-4000-8000-%'
	`).Error)
	s.Require().NoError(s.tx.Exec(`
		DELETE FROM lightwell_vulnerability_customers
		WHERE vulnerability_uuid::text LIKE '00000000-0000-4000-8000-%'
	`).Error)
	s.Require().NoError(s.tx.Exec(`
		DELETE FROM lightwell_vulnerabilities
		WHERE uuid::text LIKE '00000000-0000-4000-8000-%'
	`).Error)
}
