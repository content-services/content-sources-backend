package seeds

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func (s *SeedSuite) TestSeedLightwellVulnerabilities() {
	t := s.T()
	require.NotNil(t, s.tx)

	s.deleteLightwellSeedRows()

	err := SeedLightwellVulnerabilities(s.tx)
	require.NoError(t, err)

	var vulnCount int64
	err = s.tx.Raw("SELECT COUNT(*) FROM lightwell_vulnerabilities WHERE uuid = ?", lightwellSeedSentinelUUID).Scan(&vulnCount).Error
	require.NoError(t, err)
	assert.Equal(t, int64(1), vulnCount)

	var total int64
	err = s.tx.Raw("SELECT COUNT(*) FROM lightwell_vulnerabilities WHERE uuid::text LIKE '00000000-0000-4000-8000-%'").Scan(&total).Error
	require.NoError(t, err)
	assert.Equal(t, int64(52), total)

	var tickets []string
	err = s.tx.Raw(`
		SELECT DISTINCT ticket_id
		FROM lightwell_vulnerability_support_tickets
		WHERE vulnerability_uuid::text LIKE '00000000-0000-4000-8000-%'
		ORDER BY ticket_id
	`).Scan(&tickets).Error
	require.NoError(t, err)
	assert.Equal(t, []string{"demo-tk-1", "demo-tk-2", "demo-tk-3"}, tickets)

	var stamls []string
	err = s.tx.Raw(`
		SELECT customer_id || ':' || staml
		FROM lightwell_customer_stamls
		WHERE customer_id IN ('demo-customer-1', 'demo-customer-2')
		ORDER BY customer_id, staml
	`).Scan(&stamls).Error
	require.NoError(t, err)
	assert.Equal(t, []string{
		"demo-customer-1:demo-staml-1",
		"demo-customer-1:demo-staml-2",
		"demo-customer-2:demo-staml-2",
		"demo-customer-2:demo-staml-3",
	}, stamls)

	err = SeedLightwellVulnerabilities(s.tx)
	require.NoError(t, err)
}

func (s *SeedSuite) deleteLightwellSeedRows() {
	s.Require().NoError(s.tx.Exec(`
		DELETE FROM lightwell_customer_stamls
		WHERE customer_id IN ('demo-customer-1', 'demo-customer-2')
	`).Error)
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
