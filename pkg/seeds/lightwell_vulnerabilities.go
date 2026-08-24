package seeds

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

const (
	lightwellSeedSentinelUUID = "00000000-0000-4000-8000-000000000001"
	lightwellSeedSQLRelPath   = "db/seeds/lightwell_vulnerabilities.sql"
)

// SeedLightwellVulnerabilities loads mock Lightwell rows from db/seeds/lightwell_vulnerabilities.sql.
// It is idempotent: if the sentinel seed UUID already exists, it returns without inserting.
func SeedLightwellVulnerabilities(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var count int64
		err := tx.Raw("SELECT COUNT(*) FROM lightwell_vulnerabilities WHERE uuid = ?", lightwellSeedSentinelUUID).Scan(&count).Error
		if err != nil {
			return fmt.Errorf("could not check lightwell seed data: %w", err)
		}
		if count > 0 {
			log.Info().Msg("Lightwell seed data already present, skipping")
			return nil
		}

		sqlText, err := readLightwellSeedSQL()
		if err != nil {
			return err
		}

		for _, stmt := range statementsFromSQL(sqlText) {
			if err := tx.Exec(stmt).Error; err != nil {
				return fmt.Errorf("could not apply lightwell seed data: %w", err)
			}
		}

		log.Info().Msg("Successfully seeded lightwell vulnerabilities")
		return nil
	})
}

func readLightwellSeedSQL() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("could not determine working directory: %w", err)
	}
	for {
		path := filepath.Join(dir, lightwellSeedSQLRelPath)
		data, err := os.ReadFile(path)
		if err == nil {
			return string(data), nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("could not find %s", lightwellSeedSQLRelPath)
}

func statementsFromSQL(sqlText string) []string {
	var stmts []string
	for _, part := range strings.Split(sqlText, ";") {
		s := stripSQLComments(part)
		if s == "" || strings.EqualFold(s, "BEGIN") || strings.EqualFold(s, "COMMIT") {
			continue
		}
		stmts = append(stmts, s)
	}
	return stmts
}

func stripSQLComments(part string) string {
	var lines []string
	for _, line := range strings.Split(part, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			continue
		}
		lines = append(lines, trimmed)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
