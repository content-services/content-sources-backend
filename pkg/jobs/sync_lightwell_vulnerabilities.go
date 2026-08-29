package jobs

import (
	"context"

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

	logEvent := log.Info()
	if summary.Failed > 0 {
		logEvent = log.Warn()
	}
	logEvent.
		Int("inserted", summary.Inserted).
		Int("updated", summary.Updated).
		Int("unchanged", summary.Unchanged).
		Int("failed", summary.Failed).
		Msg("Finished syncing Lightwell vulnerabilities from Jira")
}

func runLightwellVulnerabilitySync(ctx context.Context) (lightwellsync.SyncSummary, error) {
	jiraConfig := config.Get().Clients.Jira
	jiraClient, err := jira_client.NewAtlassianJiraClient(jiraConfig.URL, jiraConfig.User, jiraConfig.Token)
	if err != nil {
		return lightwellsync.SyncSummary{}, err
	}

	daos := dao.GetDaoRegistry(db.DB)
	ingestor := lightwellsync.NewIngestor(jiraClient, daos.LightwellVulnerability, daos.LightwellAdvisory)
	return ingestor.Sync(ctx)
}
