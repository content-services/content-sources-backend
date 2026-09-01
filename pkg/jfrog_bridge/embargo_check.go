package jfrog_bridge

import (
	"fmt"
	"regexp"
	"strings"
)

var cvePattern = regexp.MustCompile(`^CVE-\d{4}-\d{4,}$`)

// EmbargoError is a policy rejection that should not be retried.
// The Kafka offset should be committed so the consumer moves on.
type EmbargoError struct {
	msg string
}

func (e *EmbargoError) Error() string { return e.msg }

// ValidateNotificationCVEs checks the CVE list from a Kafka notification.
// Every entry must be a well-formed public CVE ID.  A mixed list such as
// ["CVE-2026-1111", "LTWL-0001"] is rejected because the non-CVE entry
// may reference an embargoed vulnerability that must not reach JFrog.
func ValidateNotificationCVEs(rem Remediation) error {
	gav := fmt.Sprintf("%s:%s:%s", rem.GroupID, rem.ArtifactID, rem.Version)

	if len(rem.CVEsFixed) == 0 {
		return &EmbargoError{msg: fmt.Sprintf(
			"embargo check: %s has no CVEs in notification", gav)}
	}

	var bad []string
	for _, id := range rem.CVEsFixed {
		if !cvePattern.MatchString(id) {
			bad = append(bad, id)
		}
	}
	if len(bad) > 0 {
		return &EmbargoError{msg: fmt.Sprintf(
			"embargo check: %s contains non-public identifiers: %s",
			gav, strings.Join(bad, ", "))}
	}

	return nil
}

// ValidateOSVRecords checks the CVE IDs inside OSV records fetched from
// the Pulp repository.  This is a separate data source from the Kafka
// notification, so it gets its own validation pass.
func ValidateOSVRecords(records []OSVRecord) error {
	var bad []string
	for _, rec := range records {
		if !cvePattern.MatchString(rec.CVEID) {
			bad = append(bad, rec.CVEID)
		}
	}
	if len(bad) > 0 {
		return &EmbargoError{msg: fmt.Sprintf(
			"embargo check: OSV records contain non-public identifiers: %s",
			strings.Join(bad, ", "))}
	}
	return nil
}
