package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
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
	domainName, err := daoReg.Domain.Fetch(ctxWithLogger, task.OrgId)
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
			return err
		}
	}

	for _, snapUUID := range ds.payload.SnapshotsUUIDs {
		templateUpdateMap := make(map[string]models.Snapshot)
		snap, err := ds.daoReg.Snapshot.FetchModel(ds.ctx, snapUUID, true)
		if err != nil {
			return err
		}
		repo, err := ds.daoReg.RepositoryConfig.Fetch(ds.ctx, ds.orgID, snap.RepositoryConfigurationUUID)
		if err != nil {
			return err
		}

		err = ds.updateTemplatesUsingSnap(&templateUpdateMap, snap)
		if err != nil {
			return err
		}
		err = ds.daoReg.Template.DeleteTemplateSnapshot(ds.ctx, snap.UUID)
		if err != nil {
			return err
		}
		if repo.LastSnapshotUUID == snapUUID {
			err = ds.updateRepoConfig(repo.UUID, snapUUID)
			if err != nil {
				return err
			}
		}

		err = ds.deleteOrUpdatePulpContent(snap, repo, templateUpdateMap)
		if err != nil {
			return err
		}

		err = ds.daoReg.Snapshot.Delete(ds.ctx, snapUUID)
		if err != nil {
			return err
		}
	}

	return nil
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
			return err
		}

		// Get the stored distribution href from DB for comparison
		storedDistHref, dbErr := ds.daoReg.Template.GetDistributionHref(ds.ctx, templateUUID, repo.UUID)
		if dbErr != nil {
			logger.Warn().
				Err(dbErr).
				Str("template_uuid", templateUUID).
				Msg("Could not get stored distribution href from DB")
		}

		logger.Info().
			Str("template_uuid", templateUUID).
			Str("calculated_path", distPath).
			Str("calculated_name", distName).
			Str("stored_href", storedDistHref).
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
			return err
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
			return err
		}
	}

	latestPathIdent := fmt.Sprintf("%v/%v", repo.UUID, "latest")
	latestDistro, err := ds.getPulpClient().FindDistributionByPath(ds.ctx, latestPathIdent)
	if err != nil {
		return err
	}
	if latestDistro != nil {
		latestSnap, err := ds.daoReg.Snapshot.FetchLatestSnapshotModel(ds.ctx, repo.UUID)
		if err != nil {
			return err
		}
		_, _, err = ds.pulpDistHelper.CreateOrUpdateDistribution(repo, latestSnap.PublicationHref, repo.UUID, latestPathIdent)
		if err != nil {
			return err
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
		return err
	}
	if deleteDistributionHref != nil {
		_, err = ds.getPulpClient().PollTask(ds.ctx, *deleteDistributionHref)
		if err != nil {
			return err
		}
	}

	// Before deleting the version, list all distributions to see if any still reference this publication
	logger.Info().
		Str("snapshot_uuid", snap.UUID).
		Str("publication_href", snap.PublicationHref).
		Msg("Checking for distributions still using publication before deleting version")

	allDistributions, listErr := ds.getPulpClient().ListDistributions(ds.ctx, ds.domainName)
	if listErr != nil {
		logger.Warn().
			Err(listErr).
			Msg("Could not list distributions before version delete")
	} else if allDistributions != nil {
		blockingDistributions := []string{}
		for _, dist := range *allDistributions {
			if dist.Publication.IsSet() {
				pub := dist.Publication.Get()
				if pub != nil && *pub == snap.PublicationHref {
					blockingDistributions = append(blockingDistributions, fmt.Sprintf("%s (path: %s)", dist.Name, dist.BasePath))
					logger.Warn().
						Str("dist_name", dist.Name).
						Str("dist_base_path", dist.BasePath).
						Str("dist_href", *dist.PulpHref).
						Str("publication", *pub).
						Msg("Found distribution still using publication about to be deleted")
				}
			}
		}
		if len(blockingDistributions) > 0 {
			logger.Warn().
				Str("snapshot_uuid", snap.UUID).
				Str("publication_href", snap.PublicationHref).
				Msg("Distributions still using publication - version delete will likely fail")
		} else {
			logger.Info().
				Str("snapshot_uuid", snap.UUID).
				Msg("No distributions found using publication - safe to delete version")
		}
	}

	logger.Info().
		Str("snapshot_uuid", snap.UUID).
		Str("version_href", snap.VersionHref).
		Msg("Deleting repository version")

	deleteVersionHref, err := ds.getPulpClient().DeleteRpmRepositoryVersion(ds.ctx, snap.VersionHref)
	if err != nil {
		distributions, listDistErr := ds.getPulpClient().ListDistributions(ds.ctx, ds.domainName)
		if listDistErr != nil {
			return listDistErr
		}
		logger.Debug().Msgf("Checking distributions for publication: %v", snap.PublicationHref)
		if distributions != nil {
			for _, dist := range *distributions {
				if dist.Publication.IsSet() {
					pub := dist.Publication.Get()
					if dist.PulpHref != nil && pub != nil && *pub == snap.PublicationHref {
						logger.Debug().
							Str("href", *dist.PulpHref).
							Str("base_path", dist.BasePath).
							Str("publication", *pub).
							Msg("Found distribution using deleted publication")
					}
				}
			}
		}
		return err
	}
	if deleteVersionHref != nil {
		_, err = ds.getPulpClient().PollTask(ds.ctx, *deleteVersionHref)
		if err != nil {
			return err
		}
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
