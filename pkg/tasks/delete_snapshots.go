package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/event"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/tasks/helpers"
	"github.com/content-services/content-sources-backend/pkg/tasks/payloads"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	"github.com/content-services/content-sources-backend/pkg/utils"
)

type DeleteSnapshots struct {
	orgID               string
	rhDomainName        string
	domainName          string
	communityDomainName string
	ctx                 context.Context
	payload             *payloads.DeleteSnapshotsPayload
	task                *models.TaskInfo
	daoReg              *dao.DaoRegistry
	pulpClient          *pulp_client.PulpClient
	pulpDistHelper      *helpers.PulpDistributionHelper
}

func DeleteSnapshotsHandler(ctx context.Context, task *models.TaskInfo, _ *queue.Queue) error {
	opts := payloads.DeleteSnapshotsPayload{}
	if err := json.Unmarshal(task.Payload, &opts); err != nil {
		return fmt.Errorf("payload incorrect type for %s", config.DeleteSnapshotsTask)
	}

	logger := LogForTask(task.Id.String(), task.Typename, task.RequestID)
	ctxWithLogger := logger.WithContext(ctx)

	daoReg := dao.GetDaoRegistry(db.DB)
	domainName, err := daoReg.Domain.FetchOrCreateDomain(ctxWithLogger, task.OrgId)
	if err != nil {
		return err
	}

	rhDomainName, err := daoReg.Domain.Fetch(ctxWithLogger, config.RedHatOrg)
	if err != nil {
		return err
	}

	communityDomainName, err := daoReg.Domain.Fetch(ctxWithLogger, config.CommunityOrg)
	if err != nil {
		return err
	}

	pulpClient := pulp_client.GetPulpClientWithDomain(domainName)

	ds := DeleteSnapshots{
		orgID:               task.OrgId,
		rhDomainName:        rhDomainName,
		domainName:          domainName,
		communityDomainName: communityDomainName,
		ctx:                 ctx,
		payload:             &opts,
		task:                task,
		daoReg:              daoReg,
		pulpClient:          &pulpClient,
	}
	return ds.Run()
}

func (ds *DeleteSnapshots) Run() error {
	if !config.PulpConfigured() {
		return errors.New("no pulp client configured, can't proceed with snapshot deletion")
	} else if config.PulpConfigured() && ds.pulpClient != nil {
		err := ds.configurePulpClient()
		if err != nil {
			return fmt.Errorf("failed to configure pulp client: %w", err)
		}
	}

	logger := LogForTask(ds.task.Id.String(), ds.task.Typename, ds.task.RequestID)
	var errs []error
	for _, snapUUID := range ds.payload.SnapshotsUUIDs {
		templateUpdateMap := make(map[string]models.Snapshot)
		snap, err := ds.daoReg.Snapshot.FetchModel(ds.ctx, snapUUID, true)
		if err != nil {
			var daoErr *ce.DaoError
			if errors.As(err, &daoErr) && daoErr.NotFound {
				logger.Warn().
					Str("snapshot_uuid", snapUUID).
					Msg("snapshot not found, skipping (already deleted)")
				continue
			}
			errs = append(errs, fmt.Errorf("failed to fetch snapshot %v: %w", snapUUID, err))
			continue
		}
		repo, err := ds.daoReg.RepositoryConfig.Fetch(ds.ctx, ds.orgID, snap.RepositoryConfigurationUUID)
		if err != nil {
			var daoErr *ce.DaoError
			if errors.As(err, &daoErr) && daoErr.NotFound {
				logger.Warn().
					Str("repo_config_uuid", snap.RepositoryConfigurationUUID).
					Str("snapshot_uuid", snapUUID).
					Msg("repo config not found for snapshot, skipping")
				continue
			}
			errs = append(errs, fmt.Errorf("couldn't fetch repo config %v for snapshot %v: %w", snap.RepositoryConfigurationUUID, snapUUID, err))
			continue
		}

		if err = ds.updateTemplatesUsingSnap(&templateUpdateMap, snap); err != nil {
			errs = append(errs, fmt.Errorf("couldn't update templates for snapshot %v: %w", snapUUID, err))
			continue
		}
		if err = ds.daoReg.Template.DeleteTemplateSnapshot(ds.ctx, snap.UUID); err != nil {
			errs = append(errs, fmt.Errorf("failed to delete template snapshot associations for snapshot %v: %w", snapUUID, err))
			continue
		}
		if repo.LastSnapshotUUID == snapUUID {
			if err = ds.updateRepoConfig(repo.UUID, snapUUID); err != nil {
				errs = append(errs, fmt.Errorf("failed to update repo config for snapshot %v: %w", snapUUID, err))
				continue
			}
		}

		if err = ds.deleteOrUpdatePulpContent(snap, repo, templateUpdateMap); err != nil {
			errs = append(errs, fmt.Errorf("failed to delete pulp content for snapshot %v: %w", snapUUID, err))
			continue
		}

		if err = ds.daoReg.Snapshot.Delete(ds.ctx, snapUUID); err != nil {
			var daoErr *ce.DaoError
			if errors.As(err, &daoErr) && daoErr.NotFound {
				logger.Warn().
					Str("snapshot_uuid", snapUUID).
					Msg("snapshot not found during deletion, already deleted")
			} else {
				errs = append(errs, fmt.Errorf("failed to delete snapshot %v: %w", snapUUID, err))
			}
		}
	}

	return errors.Join(errs...)
}

func (ds *DeleteSnapshots) getPulpClient() pulp_client.PulpClient {
	return *ds.pulpClient
}

func (ds *DeleteSnapshots) configurePulpClient() error {
	repo, err := ds.daoReg.RepositoryConfig.Fetch(ds.ctx, ds.orgID, ds.payload.RepoUUID)
	if err != nil {
		return err
	}
	if repo.OrgID == config.RedHatOrg && ds.rhDomainName != "" {
		ds.pulpClient = utils.Ptr(ds.getPulpClient().WithDomain(ds.rhDomainName))
	} else if repo.OrgID == config.CommunityOrg && ds.communityDomainName != "" {
		ds.pulpClient = utils.Ptr(ds.getPulpClient().WithDomain(ds.communityDomainName))
	} else if ds.domainName != "" {
		ds.pulpClient = utils.Ptr(ds.getPulpClient().WithDomain(ds.domainName))
	}

	ds.pulpDistHelper = helpers.NewPulpDistributionHelper(ds.ctx, *ds.pulpClient)

	return nil
}

func (ds *DeleteSnapshots) deleteOrUpdatePulpContent(snap models.Snapshot, repo api.RepositoryResponse, templateUpdateMap map[string]models.Snapshot) error {
	logger := LogForTask(ds.task.Id.String(), ds.task.Typename, ds.task.RequestID)

	logger.Info().
		Str("snapshot_uuid", snap.UUID).
		Str("snapshot_publication", snap.PublicationHref).
		Int("template_count", len(templateUpdateMap)).
		Msg("Starting deleteOrUpdatePulpContent")

	for templateUUID, snaps := range templateUpdateMap {
		distPath, distName, err := getDistPathAndName(repo, templateUUID)
		if err != nil {
			return fmt.Errorf("failed to get distribution path/name for template %v: %w", templateUUID, err)
		}

		// Get the stored distribution href from DB for comparison
		storedDistHref, dbErr := ds.daoReg.Template.GetDistributionHref(ds.ctx, templateUUID, repo.UUID)
		if dbErr != nil {
			logger.Warn().
				Err(dbErr).
				Str("template_uuid", templateUUID).
				Msg("Could not get stored distribution href from DB")
		}

		if storedDistHref == nil {
			logger.Warn().Str("template_uuid", templateUUID).Msg("distribution href is null")
			continue
		}

		logger.Info().
			Str("template_uuid", templateUUID).
			Str("calculated_path", distPath).
			Str("calculated_name", distName).
			Str("stored_href", *storedDistHref).
			Str("new_publication", snaps.PublicationHref).
			Str("old_publication", snap.PublicationHref).
			Msg("Updating template distribution")

		// Check if distribution exists at calculated path
		existingDist, err := ds.getPulpClient().FindDistributionByPath(ds.ctx, distPath)
		if err != nil {
			logger.Error().
				Err(err).
				Str("dist_path", distPath).
				Msg("Error finding distribution by path")
			return fmt.Errorf("failed to find distribution by path %v for template %v: %w", distPath, templateUUID, err)
		}
		if existingDist == nil {
			logger.Warn().
				Str("template_uuid", templateUUID).
				Str("dist_path", distPath).
				Msg("Distribution not found at calculated path")
		} else if existingDist.Publication.IsSet() && existingDist.Publication.Get() != nil {
			logger.Info().
				Str("template_uuid", templateUUID).
				Str("existing_publication", *existingDist.Publication.Get()).
				Bool("matches_old_snap", *existingDist.Publication.Get() == snap.PublicationHref).
				Msg("Found existing distribution")
		}

		_, _, err = ds.pulpDistHelper.CreateOrUpdateDistribution(repo, snaps.PublicationHref, distName, distPath)
		if err != nil {
			return fmt.Errorf("failed to update distribution for template %v: %w", templateUUID, err)
		}
	}

	latestPathIdent := fmt.Sprintf("%v/%v", repo.UUID, "latest")
	latestDistro, err := ds.getPulpClient().FindDistributionByPath(ds.ctx, latestPathIdent)
	if err != nil {
		return fmt.Errorf("failed to find latest distribution by path %v: %w", latestPathIdent, err)
	}
	if latestDistro != nil {
		latestSnap, err := ds.daoReg.Snapshot.FetchLatestSnapshotModel(ds.ctx, repo.UUID)
		if err != nil {
			return fmt.Errorf("failed to fetch latest snapshot for repo %v: %w", repo.UUID, err)
		}
		_, _, err = ds.pulpDistHelper.CreateOrUpdateDistribution(repo, latestSnap.PublicationHref, repo.UUID, latestPathIdent)
		if err != nil {
			return fmt.Errorf("failed to update latest distribution for repo %v: %w", repo.UUID, err)
		}
	}

	logger.Info().
		Str("snapshot_uuid", snap.UUID).
		Str("distribution_href", snap.DistributionHref).
		Msg("Deleting snapshot's distribution")

	deleteDistributionHref, err := ds.getPulpClient().DeleteRpmDistribution(ds.ctx, snap.DistributionHref)
	if err != nil {
		logger.Warn().
			Err(err).
			Str("distribution_href", snap.DistributionHref).
			Msg("Error deleting snapshot distribution")
		return fmt.Errorf("failed to delete snapshot distribution %v: %w", snap.DistributionHref, err)
	}
	if deleteDistributionHref != nil {
		_, err = ds.getPulpClient().PollTask(ds.ctx, *deleteDistributionHref)
		if err != nil {
			return fmt.Errorf("error polling snapshot distribution deletion task for %v: %w", snap.DistributionHref, err)
		}
	}

	if cleanupErr := ds.deleteDistributionAtPath(snap.DistributionPath); cleanupErr != nil {
		return cleanupErr
	}

	if err = ds.deleteRepositoryVersion(snap); err != nil {
		return err
	}

	return nil
}

func isRepositoryVersionBlockedByDistribution(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "currently being used to distribute content") ||
		strings.Contains(msg, "update the necessary distributions first")
}

func (ds *DeleteSnapshots) deleteDistributionAtPath(distPath string) error {
	logger := LogForTask(ds.task.Id.String(), ds.task.Typename, ds.task.RequestID)

	dist, err := ds.getPulpClient().FindDistributionByPath(ds.ctx, distPath)
	if err != nil {
		return fmt.Errorf("failed to find distribution at base path %v: %w", distPath, err)
	}
	if dist == nil || dist.PulpHref == nil {
		logger.Warn().
			Str("base_path", distPath).
			Bool("distribution_found", dist != nil).
			Msg("no distribution found at path")
		return nil
	}

	logger.Info().
		Str("dist_href", *dist.PulpHref).
		Str("base_path", dist.BasePath).
		Msg("Deleting distribution at path")

	deleteDistributionHref, err := ds.getPulpClient().DeleteRpmDistribution(ds.ctx, *dist.PulpHref)
	if err != nil {
		return fmt.Errorf("failed to delete distribution %v at path %v: %w", *dist.PulpHref, distPath, err)
	}
	if deleteDistributionHref != nil {
		if _, err = ds.getPulpClient().PollTask(ds.ctx, *deleteDistributionHref); err != nil {
			return fmt.Errorf("error polling distribution deletion task for %v: %w", *dist.PulpHref, err)
		}
	}

	return nil
}

func (ds *DeleteSnapshots) deleteDistributionsBlockingPublication(snap models.Snapshot, publicationHref string) error {
	logger := LogForTask(ds.task.Id.String(), ds.task.Typename, ds.task.RequestID)

	dist, err := ds.getPulpClient().FindDistributionByPath(ds.ctx, snap.DistributionPath)
	if err != nil {
		return fmt.Errorf("failed to find distribution at base path %v: %w", snap.DistributionPath, err)
	}
	if dist == nil || dist.PulpHref == nil {
		return nil
	}
	if !dist.Publication.IsSet() {
		return ds.deleteDistributionAtPath(snap.DistributionPath)
	}
	pub := dist.Publication.Get()
	if pub == nil || *pub != publicationHref {
		return nil
	}

	logger.Info().
		Str("snapshot_uuid", snap.UUID).
		Str("dist_href", *dist.PulpHref).
		Str("base_path", dist.BasePath).
		Str("publication", publicationHref).
		Msg("Deleting untracked distribution blocking repository version deletion")

	return ds.deleteDistributionAtPath(snap.DistributionPath)
}

func (ds *DeleteSnapshots) deleteRepositoryVersion(snap models.Snapshot) error {
	logger := LogForTask(ds.task.Id.String(), ds.task.Typename, ds.task.RequestID).With().
		Str("snapshot_uuid", snap.UUID).
		Str("version_href", snap.VersionHref).
		Logger()

	logger.Info().Msg("Deleting repository version")

	deleteVersionHref, err := ds.getPulpClient().DeleteRpmRepositoryVersion(ds.ctx, snap.VersionHref)
	if err != nil {
		logger.Error().Err(err).Msg("failed to delete repository version")
		return fmt.Errorf("failed to delete repository version: %w", err)
	}

	if deleteVersionHref != nil {
		_, err = ds.getPulpClient().PollTask(ds.ctx, *deleteVersionHref)
		if err != nil {
			if !isRepositoryVersionBlockedByDistribution(err) {
				logger.Error().Err(err).Msg("error polling repository version deletion task")
				return fmt.Errorf("error polling repository version deletion task: %w", err)
			}
			if cleanupErr := ds.deleteDistributionsBlockingPublication(snap, snap.PublicationHref); cleanupErr != nil {
				logger.Error().Err(err).AnErr("cleanup_error", cleanupErr).
					Msg("error polling repository version deletion task; also failed to clean up blocking distributions")
				return fmt.Errorf(
					"error polling repository version deletion task: %w (also failed to clean up blocking distributions: %v)",
					err, cleanupErr,
				)
			}
			deleteVersionHref, err = ds.getPulpClient().DeleteRpmRepositoryVersion(ds.ctx, snap.VersionHref)
			if err != nil {
				logger.Error().Err(err).Msg("failed to delete repository version after cleanup")
				return fmt.Errorf("failed to delete repository version after cleanup: %w", err)
			}
			if deleteVersionHref != nil {
				_, err = ds.getPulpClient().PollTask(ds.ctx, *deleteVersionHref)
				if err != nil {
					logger.Error().Err(err).Msg("error polling repository version deletion task after cleanup")
					return fmt.Errorf("error polling repository version deletion task after cleanup: %w", err)
				}
			}
		}
	} else if _, verifyErr := ds.getPulpClient().GetRpmRepositoryVersion(ds.ctx, snap.VersionHref); verifyErr == nil {
		logger.Warn().Msg("repository version still exists after delete returned no task")
		return fmt.Errorf("repository version still exists after delete returned no task")
	}

	return nil
}

func (ds *DeleteSnapshots) updateTemplatesUsingSnap(templateUpdateMap *map[string]models.Snapshot, snap models.Snapshot) (err error) {
	logger := LogForTask(ds.task.Id.String(), ds.task.Typename, ds.task.RequestID)

	logger.Info().
		Str("snapshot_uuid", snap.UUID).
		Str("repo_uuid", snap.RepositoryConfigurationUUID).
		Str("org_id", ds.orgID).
		Bool("is_shared_repo", ds.orgID == config.RedHatOrg || ds.orgID == config.CommunityOrg).
		Msg("Looking for templates using snapshot")

	repoUUIDs := []string{snap.RepositoryConfigurationUUID}
	var templates []api.TemplateResponse
	if ds.orgID == config.RedHatOrg || ds.orgID == config.CommunityOrg {
		templates, err = ds.daoReg.Template.InternalOnlyGetTemplatesForSnapshots(ds.ctx, []string{snap.UUID})
		if err != nil {
			return err
		}
		logger.Info().
			Str("snapshot_uuid", snap.UUID).
			Int("template_count", len(templates)).
			Msg("Found templates using InternalOnlyGetTemplatesForSnapshots (shared repo)")
	} else {
		templateResponse, count, err := ds.daoReg.Template.List(ds.ctx, ds.orgID, false, api.PaginationData{Limit: -1}, api.TemplateFilterData{
			SnapshotUUIDs: []string{snap.UUID},
		})
		if err != nil {
			return err
		}
		logger.Info().
			Str("snapshot_uuid", snap.UUID).
			Int("template_count", int(count)).
			Msg("Found templates using Template.List")
		if count == 0 {
			return nil
		}
		templates = templateResponse.Data
	}

	for _, template := range templates {
		date := template.Date
		if template.UseLatest {
			date = time.Now()
		}

		logger.Info().
			Str("template_uuid", template.UUID).
			Str("template_name", template.Name).
			Str("template_org_id", template.OrgID).
			Time("template_date", date).
			Bool("use_latest", template.UseLatest).
			Msg("Finding replacement snapshot for template")

		snaps, err := ds.daoReg.Snapshot.FetchSnapshotsModelByDateAndRepository(ds.ctx, ds.orgID, api.ListSnapshotByDateRequest{
			RepositoryUUIDS: repoUUIDs,
			Date:            date,
		})
		if err != nil {
			logger.Warn().
				Err(err).
				Str("template_uuid", template.UUID).
				Msg("Error fetching replacement snapshots")
			return err
		}
		if len(snaps) != 1 {
			logger.Warn().
				Str("template_uuid", template.UUID).
				Int("snapshots_found", len(snaps)).
				Msg("Expected exactly 1 replacement snapshot but found different count")
			return errors.New("no other snapshot was found")
		}

		logger.Info().
			Str("template_uuid", template.UUID).
			Str("replacement_snapshot_uuid", snaps[0].UUID).
			Str("replacement_publication", snaps[0].PublicationHref).
			Msg("Found replacement snapshot, updating template")

		err = ds.daoReg.Template.UpdateSnapshots(ds.ctx, template.UUID, repoUUIDs, snaps)
		if err != nil {
			return err
		}

		(*templateUpdateMap)[template.UUID] = snaps[0]

		event.SendTemplateEvent(template.OrgID, event.TemplateUpdated, []event.TemplateEvent{event.MapTemplateResponse(template)})
	}

	return nil
}

func (ds *DeleteSnapshots) updateRepoConfig(repoUUID, snapUUID string) error {
	latestSnap, err := ds.daoReg.Snapshot.FetchLatestSnapshotModel(ds.ctx, repoUUID)
	if err != nil {
		return err
	}
	if latestSnap.UUID == snapUUID {
		return errors.New("no other snapshot was found")
	}

	err = ds.daoReg.RepositoryConfig.UpdateLastSnapshot(ds.ctx, ds.orgID, repoUUID, latestSnap.UUID)
	if err != nil {
		return err
	}
	err = ds.daoReg.RepositoryConfig.UpdateLastSnapshotTask(ds.ctx, "", ds.orgID, repoUUID)
	if err != nil {
		return err
	}

	return nil
}
