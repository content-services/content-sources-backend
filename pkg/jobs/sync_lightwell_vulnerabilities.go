package jobs

import (
	"context"
	"time"

	"github.com/content-services/content-sources-backend/pkg/clients/jira_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	lightwellsync "github.com/content-services/content-sources-backend/pkg/lightwell/sync"
	"github.com/rs/zerolog/log"
)

func SyncLightwellVulnerabilities(_ []string) {
	summary, err := runLightwellVulnerabilitySync(context.Background())
	if err != nil {
		log.Fatal().Err(err).Msg("Lightwell vulnerability sync failed")
	}

	for _, failure := range summary.Failures {
		log.Error().Msg(failure)
	}
	for _, warning := range summary.Warnings {
		log.Warn().Msg(warning)
	}

	logEvent := log.Info()
	if summary.Failed > 0 {
		logEvent = log.Warn()
	}
	logEvent.
		Int("inserted", summary.Inserted).
		Int("updated", summary.Updated).
		Int("unchanged", summary.Unchanged).
		Int("deleted", summary.Deleted).
		Int("failed", summary.Failed).
		Int("on_hold_without_reason", summary.OnHoldWithoutReason).
		Msg("Finished syncing Lightwell vulnerabilities from Jira")
}

func runLightwellVulnerabilitySync(ctx context.Context) (lightwellsync.SyncSummary, error) {
	startedAt := time.Now().UTC()

	jiraConfig := config.Get().Clients.Jira
	jiraClient, err := jira_client.NewAtlassianJiraClient(jiraConfig.URL, jiraConfig.User, jiraConfig.Token)
	if err != nil {
		return lightwellsync.SyncSummary{}, err
	}

	daos := dao.GetDaoRegistry(db.DB)
	verifier := publicationPackageVerifier(daos)

	ingestor := lightwellsync.NewIngestor(
		jiraClient,
		daos.LightwellVulnerability,
		daos.LightwellAdvisory,
		verifier,
		lightwellsync.WithPromoteByJiraState(config.Get().Options.LightwellPromoteByJiraState),
	)
	summary, err := ingestor.Sync(ctx)
	if err != nil {
		return summary, err
	}
	if err := daos.LightwellVulnerability.RecordSuccessfulSync(ctx, startedAt); err != nil {
		return summary, err
	}
	return summary, nil
}

func publicationPackageVerifier(daos *dao.DaoRegistry) lightwellsync.PublicationPackageVerifier {
	return lightwellsync.NewPublicationPackageVerifier(daos.RepositoryConfig, daos.LightwellPackage)
}
