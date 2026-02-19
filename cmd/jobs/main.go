package main

import (
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/jobs"
	"github.com/rs/zerolog/log"
)

type jobFunc func([]string)

func loadJobs() map[string]jobFunc {
	return map[string]jobFunc{
		"retry-failed-tasks":               jobs.RetryFailedTasks,
		"create-latest-distributions":      jobs.CreateLatestDistributions,
		"transform-pulp-logs":              jobs.TransformPulpLogs,
		"cleanup-missing-domains":          jobs.CleanupMissingDomains,
		"set-domain-label":                 jobs.SetDomainLabel,
		"set-detected-os-versions":         jobs.SetDetectedOSVersions,
		"cancel-snapshot-delete-tasks":     jobs.CancelSnapshotDeleteTasks,
		"disable-snapshot-for-epel-repos":  jobs.DisableSnapshotForEpelRepos,
		"delete-invalid-redhat-repos":      jobs.DeleteInvalidRedHatRepos,
		"migrate-templates-to-shared-epel": jobs.MigrateTemplatesToSharedEpel,
	}
}

func usage() {
	jobNames := slices.Collect(maps.Keys(loadJobs()))
	log.Warn().Msgf("Usage: go run cmd/jobs/main.go  $JOB_NAME\n  (Possible jobs: %v)", strings.Join(jobNames, ", "))
	os.Exit(-1)
}

func main() {
	config.Load()
	config.ConfigureLogging()
	err := db.Connect()
	if err != nil {
		log.Panic().Err(err).Msg("Failed to connect to database")
	}
	args := os.Args
	if len(args) < 2 {
		usage()
	}
	job, ok := loadJobs()[args[1]]
	if ok {
		job(args[2:])
	} else {
		usage()
	}
}
