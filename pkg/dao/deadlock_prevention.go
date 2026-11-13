package dao

import (
	"slices"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// createRpmsSorted inserts RPMs with automatic sorting by checksum to prevent deadlocks.
//
// When multiple transactions insert overlapping RPM sets concurrently, they can deadlock
// if they acquire row locks in different orders. Sorting by checksum ensures all transactions
// lock rows in the same order, eliminating circular wait conditions.
//
// Use this instead of db.Create() for batch RPM inserts during introspection.
func createRpmsSorted(db *gorm.DB, rpms []models.Rpm) *gorm.DB {
	if len(rpms) == 0 {
		return db
	}

	// Sort by checksum (unique constraint) to ensure consistent lock ordering
	slices.SortFunc(rpms, func(a, b models.Rpm) int {
		return strings.Compare(a.Checksum, b.Checksum)
	})

	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "checksum"}},
		DoNothing: true,
	}).Create(&rpms)
}

// createRepositoryRpmsSorted inserts repository-RPM associations with automatic sorting
// by rpm_uuid to prevent deadlocks.
//
// When multiple repositories share packages and introspect concurrently, unsorted inserts
// can cause deadlocks. Sorting by rpm_uuid ensures consistent lock ordering.
//
// Use this instead of db.Create() for batch RepositoryRpm inserts during introspection.
func createRepositoryRpmsSorted(db *gorm.DB, assocs []models.RepositoryRpm) *gorm.DB {
	if len(assocs) == 0 {
		return db
	}

	// Sort by rpm_uuid (part of composite primary key) to ensure consistent lock ordering
	slices.SortFunc(assocs, func(a, b models.RepositoryRpm) int {
		return strings.Compare(a.RpmUUID, b.RpmUUID)
	})

	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "repository_uuid"}, {Name: "rpm_uuid"}},
		DoNothing: true,
	}).Create(&assocs)
}
