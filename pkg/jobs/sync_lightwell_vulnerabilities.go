package jobs

import (
	"context"
	"fmt"

	"github.com/content-services/content-sources-backend/pkg/clients/jira_client"
	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	lightwellsync "github.com/content-services/content-sources-backend/pkg/lightwell/sync"
	"github.com/content-services/tang/pkg/tangy"
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
		Msg("Finished syncing Lightwell vulnerabilities from Jira")
}

func runLightwellVulnerabilitySync(ctx context.Context) (lightwellsync.SyncSummary, error) {
	jiraConfig := config.Get().Clients.Jira
	jiraClient, err := jira_client.NewAtlassianJiraClient(jiraConfig.URL, jiraConfig.User, jiraConfig.Token)
	if err != nil {
		return lightwellsync.SyncSummary{}, err
	}

	daos := dao.GetDaoRegistry(db.DB)
	verifier, closeTang, warning := publicationPackageVerifier(daos)
	if closeTang != nil {
		defer closeTang()
	}

	ingestor := lightwellsync.NewIngestor(
		jiraClient,
		daos.LightwellVulnerability,
		daos.LightwellAdvisory,
		verifier,
	)
	summary, err := ingestor.Sync(ctx)
	if warning != "" {
		summary.Warnings = append([]string{warning}, summary.Warnings...)
	}
	return summary, err
}

func publicationPackageVerifier(daos *dao.DaoRegistry) (lightwellsync.PublicationPackageVerifier, func(), string) {
	pulpConfig := config.Get().Clients.Pulp
	if pulpConfig.Server == "" {
		return nil, nil, "package publication verification disabled: Pulp is not configured"
	}

	tang, err := tangy.New(tangy.Database{
		Name:       pulpConfig.Database.Name,
		Host:       pulpConfig.Database.Host,
		Port:       pulpConfig.Database.Port,
		User:       pulpConfig.Database.User,
		Password:   pulpConfig.Database.Password,
		CACertPath: pulpConfig.Database.CACertPath,
		PoolLimit:  pulpConfig.Database.PoolLimit,
	}, tangy.Logger{
		Logger:   &log.Logger,
		LogLevel: config.Get().Logging.Level,
		Enabled:  true,
	})
	if err != nil {
		return nil, nil, fmt.Sprintf("package publication verification disabled: cannot initialize Tang: %v", err)
	}

	pulp := pulp_client.GetPulpClientWithDomain(config.LightwellDomainName)
	verifier := lightwellsync.NewPublicationPackageVerifier(daos.RepositoryConfig, pulp, tang)
	return verifier, tang.Close, ""
}
