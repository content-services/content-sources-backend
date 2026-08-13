package commands

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/event"
	"github.com/content-services/content-sources-backend/pkg/external_repos"
	vp "github.com/content-services/content-sources-backend/pkg/external_repos/vulnerability_parser"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v2"
	"gorm.io/gorm"
)

const maxResponseBytes = 100 * 1024 * 1024 // 100 MB
const maxConcurrentFetches = 10

func SyncLightwellAdvisoriesAction(c *cli.Context) error {
	ctx := c.Context
	force := c.Bool("force")
	err := syncLightwellAdvisories(ctx, db.DB, force)
	if err != nil {
		log.Error().Err(err).Msg("Failed to sync lightwell advisories")
		return err
	}
	log.Info().Msg("Successfully synced lightwell advisories.")
	return nil
}

func syncLightwellAdvisories(ctx context.Context, database *gorm.DB, force bool) error {
	daoReg := dao.GetDaoRegistry(database)

	lightwellEntries, err := external_repos.LoadLightwellAllowlist()
	if err != nil {
		return fmt.Errorf("error loading lightwell allowlist: %w", err)
	}

	httpClient := &http.Client{Timeout: 90 * time.Second}

	var errs []error
	for _, entry := range lightwellEntries {
		if entry.OsvPath == "" {
			continue
		}

		err := processOSVForEntry(ctx, daoReg, httpClient, entry, force)
		if err != nil {
			log.Error().Err(err).
				Str("entry", entry.Name).
				Str("osv_path", entry.OsvPath).
				Msg("Failed to process OSV entry, continuing with next")
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

func processOSVForEntry(
	ctx context.Context,
	daoReg *dao.DaoRegistry,
	httpClient *http.Client,
	entry external_repos.LightwellAllowlistEntry,
	force bool,
) error {
	logger := log.With().Str("entry", entry.Name).Str("osv_path", entry.OsvPath).Logger()

	baseURL, err := url.JoinPath(config.Get().Clients.Pulp.ContentOrigin, config.Get().Clients.Pulp.ContentPathPrefix, entry.OsvPath)
	if err != nil {
		return fmt.Errorf("error constructing base URL: %w", err)
	}
	baseURL = strings.TrimRight(baseURL, "/")

	repoConfig, err := daoReg.RepositoryConfig.InternalOnly_FetchRepoConfigByName(ctx, config.LightwellOrg, entry.Name)
	if err != nil {
		return fmt.Errorf("error finding repository for %q: %w", entry.Name, err)
	}
	repoConfigUUID := repoConfig.UUID

	manifestURL := baseURL + "/PULP_MANIFEST"
	manifestData, err := httpGet(ctx, httpClient, manifestURL)
	if err != nil {
		return fmt.Errorf("error fetching manifest from %s: %w", manifestURL, err)
	}

	manifestEntries := vp.ParseManifest(manifestData)

	existingAdvisories, err := daoReg.LightwellAdvisory.ListByRepository(ctx, repoConfigUUID)
	if err != nil {
		return fmt.Errorf("error listing existing advisories: %w", err)
	}

	existingByChecksum := make(map[string][]dao.LightwellAdvisoryInput, len(existingAdvisories))
	for _, a := range existingAdvisories {
		existingByChecksum[a.Checksum] = append(existingByChecksum[a.Checksum], a)
	}

	advisories, updated := buildAdvisoryInputs(ctx, logger, httpClient, baseURL, manifestEntries, existingByChecksum, force)

	logger.Info().Int("total", len(advisories)).Int("updated", updated).Msg("Syncing advisories")

	if err := daoReg.LightwellAdvisory.SyncForRepository(ctx, repoConfigUUID, entry.Name, advisories); err != nil {
		return err
	}

	return sendAdvisoryNotifications(ctx, daoReg, logger, repoConfigUUID, entry.Name)
}

func sendAdvisoryNotifications(
	ctx context.Context,
	daoReg *dao.DaoRegistry,
	logger zerolog.Logger,
	repoConfigUUID string,
	repoName string,
) error {
	eventType := event.LightwellEventType(repoName)
	if eventType == "" {
		return nil
	}

	unnotified, err := daoReg.LightwellAdvisory.ListUnnotifiedAdvisories(ctx, repoConfigUUID)
	if err != nil {
		return fmt.Errorf("error listing unnotified advisories: %w", err)
	}
	if len(unnotified) == 0 {
		return nil
	}

	orgs, err := daoReg.UserPreference.ListDistinctOrgsByPreference(ctx,
		models.UserPreferenceLightwellNotificationEnabled, "true")
	if err != nil {
		return fmt.Errorf("error listing opted-in orgs: %w", err)
	}

	if len(orgs) == 0 {
		log.Debug().Msg("No orgs opted-in for lightwell notifications")
		return nil
	}

	inputs := make([]event.LightwellNotificationInput, len(unnotified))
	for i, u := range unnotified {
		inputs[i] = event.LightwellNotificationInput{
			PackageName:   u.PackageName,
			AdvisoryID:    u.AdvisoryID,
			Severity:      u.Severity,
			FixedVersions: u.FixedVersions,
			ReferenceURLs: u.ReferenceURLs,
		}
	}

	events := event.BuildLightwellNotificationEvents(inputs)
	severity := event.MaximumSeverity(inputs)

	for _, orgID := range orgs {
		event.SendLightwellNotification(orgID, eventType, severity, events)
	}

	logger.Info().Int("advisory_count", len(unnotified)).Int("org_count", len(orgs)).Msg("Sent lightwell advisory notifications")

	if err := daoReg.LightwellAdvisory.MarkAsNotified(ctx, repoConfigUUID, unnotified); err != nil {
		return fmt.Errorf("error marking advisories as notified: %w", err)
	}

	return nil
}

type fetchResult struct {
	advisories []dao.LightwellAdvisoryInput
	fetched    bool
}

func buildAdvisoryInputs(
	ctx context.Context,
	logger zerolog.Logger,
	httpClient *http.Client,
	baseURL string,
	manifestEntries []vp.ManifestEntry,
	existingByChecksum map[string][]dao.LightwellAdvisoryInput,
	force bool,
) ([]dao.LightwellAdvisoryInput, int) {
	results := make([]fetchResult, len(manifestEntries))

	var wg sync.WaitGroup
	sem := make(chan struct{}, maxConcurrentFetches)

	for i, me := range manifestEntries {
		if !force {
			if existing, ok := existingByChecksum[me.Checksum]; ok {
				results[i] = fetchResult{advisories: existing}
				continue
			}
		}

		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, entry vp.ManifestEntry) {
			defer wg.Done()
			defer func() { <-sem }()

			fileURL := baseURL + "/" + entry.Filename
			fileData, err := httpGet(ctx, httpClient, fileURL)
			if err != nil {
				logger.Warn().Err(err).Str("file", entry.Filename).Msg("Failed to fetch OSV file, skipping")
				return
			}

			infos, err := vp.ParseOSVAdvisory(fileData)
			if err != nil {
				logger.Warn().Err(err).Str("file", entry.Filename).Msg("Failed to parse OSV file, skipping")
				return
			}

			var advisories []dao.LightwellAdvisoryInput
			for _, info := range infos {
				advisories = append(advisories, dao.LightwellAdvisoryInput{
					AdvisoryID:    info.AdvisoryID,
					Severity:      info.Severity,
					Details:       info.Details,
					ReferenceURLs: info.References,
					PackageName:   info.PackageName,
					FixedVersions: info.FixedVersions,
					Checksum:      entry.Checksum,
				})
			}
			results[idx] = fetchResult{advisories: advisories, fetched: true}
		}(i, me)
	}

	wg.Wait()

	var advisories []dao.LightwellAdvisoryInput
	var updated int
	for _, r := range results {
		advisories = append(advisories, r.advisories...)
		if r.fetched {
			updated++
		}
	}
	return advisories, updated
}

func httpGet(ctx context.Context, client *http.Client, reqURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	if config.Get().Clients.Lightwell.Username != "" {
		req.SetBasicAuth(config.Get().Clients.Lightwell.Username, config.Get().Clients.Lightwell.Password)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, reqURL)
	}

	return io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
}
