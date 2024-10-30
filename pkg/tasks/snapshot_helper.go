package tasks

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/tasks/helpers"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
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

	distHref, addedContentGuard, err := helper.CreateOrUpdateDistribution(sh.orgId, *sh.payload.GetSnapshotIdent(), distPath, publicationHref)
	if err != nil {
		return err
	}

	latestPathIdent := fmt.Sprintf("%v/%v", sh.repo.UUID, "latest")

	_, _, err = helper.CreateOrUpdateDistribution(sh.orgId, sh.repo.UUID, latestPathIdent, publicationHref)
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
		ContentGuardAdded:           addedContentGuard,
	}
	sh.logger.Debug().Msgf("Snapshot created at: %v", distPath)
	err = sh.daoReg.Snapshot.Create(sh.ctx, &snap)
	if err != nil {
		return err
	}
	return nil
}

func (sh *SnapshotHelper) Cleanup() error {
	if sh.payload.GetDistributionTaskHref() != nil {
		task, err := sh.pulpClient.CancelTask(sh.ctx, *sh.payload.GetDistributionTaskHref())
		if err != nil {
			return err
		}
		task, err = sh.pulpClient.GetTask(sh.ctx, *sh.payload.GetDistributionTaskHref())
		if err != nil {
			return err
		}
		versionHref := pulp_client.SelectRpmDistributionHref(&task)
		if versionHref != nil {
			_, err = sh.pulpClient.DeleteRpmDistribution(sh.ctx, *versionHref)
			if err != nil {
				return err
			}
		}
	}
	if sh.payload.GetSnapshotIdent() != nil {
		err := sh.daoReg.Snapshot.Delete(sh.ctx, *sh.payload.GetSnapshotIdent())
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
