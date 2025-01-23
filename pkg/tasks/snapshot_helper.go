package tasks

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/tasks/helpers"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"gorm.io/gorm"
)

// ResumableSnapshotInterface used to store various references needed
//
//	for snapshotHelper to be resumable.  Typically implemented by the task using the helper
type ResumableSnapshotInterface interface {
	SavePublicationTaskHref(href string) error
	GetPublicationTaskHref() *string

	SaveDistributionTaskHref(href string) error
	GetDistributionTaskHref() *string

	SaveSnapshotIdent(id string) error
	GetSnapshotIdent() *string

	SaveSnapshotUUID(uuid string) error
	GetSnapshotUUID() *string
}

// SnapshotHelper is meant to be used by another task, and be able to turn a repository Version into a
//
//	snapshot, with everything needed in pulp (publication, distributions)
type SnapshotHelper struct {
	pulpClient pulp_client.PulpClient
	ctx        context.Context
	payload    ResumableSnapshotInterface
	logger     *zerolog.Logger
	orgId      string
	repo       api.RepositoryResponse
	daoReg     *dao.DaoRegistry
	domainName string
}

func (sh *SnapshotHelper) Run(versionHref string) error {
	publicationHref, err := sh.findOrCreatePublication(versionHref)
	if err != nil {
		return err
	}

	if sh.payload.GetSnapshotIdent() == nil {
		err = sh.payload.SaveSnapshotIdent(uuid.NewString())
		if err != nil {
			return fmt.Errorf("unable to save snapshot ident: %w", err)
		}
	}

	distPath := fmt.Sprintf("%v/%v", sh.repo.UUID, *sh.payload.GetSnapshotIdent())
	helper := helpers.NewPulpDistributionHelper(sh.ctx, sh.pulpClient)

	distHref, err := helper.CreateOrUpdateDistribution(sh.repo, publicationHref, *sh.payload.GetSnapshotIdent(), distPath)
	if err != nil {
		return err
	}
	err = sh.payload.SaveDistributionTaskHref(distHref)
	if err != nil {
		return fmt.Errorf("unable to save distribution task href: %w", err)
	}

	latestPathIdent := helpers.GetLatestRepoDistPath(sh.repo.UUID)

	_, err = helper.CreateOrUpdateDistribution(sh.repo, publicationHref, sh.repo.UUID, latestPathIdent)
	if err != nil {
		return err
	}
	version, err := sh.pulpClient.GetRpmRepositoryVersion(sh.ctx, versionHref)
	if err != nil {
		return err
	}

	if version.ContentSummary == nil {
		sh.logger.Error().Msgf("Found nil content Summary for version %v", versionHref)
	}

	current, added, removed := ContentSummaryToContentCounts(version.ContentSummary)

	snap := models.Snapshot{
		VersionHref:                 versionHref,
		PublicationHref:             publicationHref,
		DistributionPath:            distPath,
		RepositoryPath:              filepath.Join(sh.domainName, distPath),
		DistributionHref:            distHref,
		RepositoryConfigurationUUID: sh.repo.UUID,
		ContentCounts:               current,
		AddedCounts:                 added,
		RemovedCounts:               removed,
	}
	sh.logger.Debug().Msgf("Snapshot created at: %v", distPath)
	err = sh.daoReg.Snapshot.Create(sh.ctx, &snap)
	if err != nil {
		return err
	}
	err = sh.payload.SaveSnapshotUUID(snap.UUID)
	if err != nil {
		return fmt.Errorf("unable to save snapshot uuid: %w", err)
	}

	return nil
}

func (sh *SnapshotHelper) Cleanup() error {
	if distHref := sh.payload.GetDistributionTaskHref(); distHref != nil {
		deleteDistributionHref, err := sh.pulpClient.DeleteRpmDistribution(sh.ctx, *distHref)
		if err != nil {
			return err
		}
		_, err = sh.pulpClient.PollTask(sh.ctx, deleteDistributionHref)
		if err != nil {
			return err
		}

		err = sh.payload.SavePublicationTaskHref("")
		if err != nil {
			return err
		}
		err = sh.payload.SaveDistributionTaskHref("")
		if err != nil {
			return err
		}
	}

	if sh.payload.GetSnapshotUUID() != nil {
		err := sh.daoReg.Snapshot.Delete(sh.ctx, *sh.payload.GetSnapshotUUID())
		if err != nil {
			return err
		}
	}
	err := sh.payload.SaveSnapshotUUID("")
	if err != nil {
		return err
	}

	helper := helpers.NewPulpDistributionHelper(sh.ctx, sh.pulpClient)
	latestPathIdent := helpers.GetLatestRepoDistPath(sh.repo.UUID)
	latestDistro, err := sh.pulpClient.FindDistributionByPath(sh.ctx, latestPathIdent)
	if err != nil {
		return err
	}
	if latestDistro != nil {
		latestSnap, err := sh.daoReg.Snapshot.FetchLatestSnapshotModel(sh.ctx, sh.repo.UUID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				deleteDistributionHref, err := sh.pulpClient.DeleteRpmDistribution(sh.ctx, *latestDistro.PulpHref)
				if err != nil {
					return err
				}
				_, err = sh.pulpClient.PollTask(sh.ctx, deleteDistributionHref)
				if err != nil {
					return err
				}
				return nil
			}
			return err
		}

		_, _, err = helper.CreateOrUpdateDistribution(sh.repo.OrgID, latestSnap.PublicationHref, sh.repo.UUID, latestPathIdent)
		if err != nil {
			return err
		}
	}

	return nil
}

func (sh *SnapshotHelper) findOrCreatePublication(versionHref string) (string, error) {
	var publicationHref *string
	// Publication
	publication, err := sh.pulpClient.FindRpmPublicationByVersion(sh.ctx, versionHref)
	if err != nil {
		return "", err
	}
	if publication == nil || publication.PulpHref == nil {
		if sh.payload.GetPublicationTaskHref() == nil {
			publicationTaskHref, err := sh.pulpClient.CreateRpmPublication(sh.ctx, versionHref)
			if err != nil {
				return "", err
			}
			if publicationTaskHref == nil {
				return "", fmt.Errorf("publicationTaskHref cannot be nil")
			}
			err = sh.payload.SavePublicationTaskHref(*publicationTaskHref)

			if err != nil {
				return "", err
			}
		} else {
			sh.logger.Debug().Str("pulp_task_id", *sh.payload.GetPublicationTaskHref()).Msg("Resuming Publication task")
		}

		publicationTask, err := sh.pulpClient.PollTask(sh.ctx, *sh.payload.GetPublicationTaskHref())
		if err != nil {
			return "", err
		}
		publicationHref = pulp_client.SelectPublicationHref(publicationTask)
		if publicationHref == nil {
			return "", fmt.Errorf("Could not find a publication href in task: %v", publicationTask.PulpHref)
		}
	} else {
		publicationHref = publication.PulpHref
	}
	return *publicationHref, nil
}
