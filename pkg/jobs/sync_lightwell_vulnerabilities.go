package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/content-services/content-sources-backend/pkg/clients/jira_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/lightwell/db/store"
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
		Int("on_hold_from_component_upgrade", summary.OnHoldFromComponentUpgrade).
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
		lightwellVulnerabilitySyncStore{LightwellVulnerabilityDao: daos.LightwellVulnerability, queries: db.LightwellQueries},
		daos.LightwellAdvisory,
		verifier,
		lightwellsync.WithPromoteByJiraState(config.Get().Options.LightwellPromoteByJiraState),
		lightwellsync.WithPackagePublishTimes(dao.NewPackagePublishTimeLookup(db.LightwellQueries)),
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

// lightwellVulnerabilitySyncStore adds the stored stage and last_updated read
// the beacon sync needs. Save and delete stay on the vulnerability DAO.
type lightwellVulnerabilitySyncStore struct {
	dao.LightwellVulnerabilityDao
	queries store.Querier
}

func (s lightwellVulnerabilitySyncStore) ListStates(ctx context.Context) (map[string]lightwellsync.VulnerabilityState, error) {
	if s.queries == nil {
		return nil, fmt.Errorf("lightwell querier is not initialized")
	}
	rows, err := s.queries.ListVulnerabilitySyncStates(ctx)
	if err != nil {
		return nil, fmt.Errorf("list vulnerability sync states: %w", err)
	}
	states := make(map[string]lightwellsync.VulnerabilityState, len(rows))
	for _, row := range rows {
		states[row.VulnerabilityKey] = lightwellsync.VulnerabilityState{
			Stage:       row.Stage,
			LastUpdated: row.LastUpdated.UTC(),
		}
	}
	return states, nil
}
