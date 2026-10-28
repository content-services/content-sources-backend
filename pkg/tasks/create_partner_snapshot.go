package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/tasks/payloads"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	"github.com/content-services/content-sources-backend/pkg/utils"
	"github.com/rs/zerolog"
)

func CreatePartnerSnapshotHandler(ctx context.Context, task *models.TaskInfo, queue *queue.Queue) error {
	opts := payloads.CreatePartnerSnapshotPayload{}
	if err := json.Unmarshal(task.Payload, &opts); err != nil {
		return fmt.Errorf("payload incorrect type for CreatePartnerSnapshot")
	}

	logger := LogForTask(task.Id.String(), task.Typename, task.RequestID)
	daoReg := dao.GetDaoRegistry(db.DB)
	domainName := config.Get().Clients.Pulp.CoprDomain
	pulpClient := pulp_client.GetPulpClientWithDomain(domainName)

	cps := CreatePartnerSnapshot{
		orgID:      task.OrgId,
		accountID:  task.AccountId,
		domainName: domainName,
		daoReg:     daoReg,
		pulpClient: pulpClient,
		task:       task,
		payload:    &opts,
		queue:      queue,
		ctx:        ctx,
		logger:     logger,
	}

	return cps.Run()
}

type CreatePartnerSnapshot struct {
	orgID      string
	accountID  string
	domainName string
	daoReg     *dao.DaoRegistry
	pulpClient pulp_client.PulpClient
	payload    *payloads.CreatePartnerSnapshotPayload
	task       *models.TaskInfo
	queue      *queue.Queue
	ctx        context.Context
	logger     *zerolog.Logger
	repoConfig api.RepositoryResponse
}

func (cps *CreatePartnerSnapshot) Run() error {
	// Ensure the Copr domain exists in Pulp
	_, err := cps.pulpClient.LookupOrCreateDomain(cps.ctx, cps.domainName)
	if err != nil {
		return fmt.Errorf("failed to lookup or create Copr domain: %w", err)
	}

	// Get publication to extract repository version href
	publication, err := cps.pulpClient.GetRpmPublication(cps.ctx, cps.payload.PublicationHref)
	if err != nil {
		return fmt.Errorf("failed to get publication: %w", err)
	}
	if publication == nil || publication.RepositoryVersion == nil {
		return fmt.Errorf("publication not found or missing repository version")
	}
	versionHref := *publication.RepositoryVersion

	// Find or create repository configuration (by Copr repository href)
	cps.repoConfig, err = cps.findOrCreateRepoConfig()
	if err != nil {
		return fmt.Errorf("failed to find or create repository config: %w", err)
	}

	// Check if snapshot already exists
	existingSnap, err := cps.daoReg.Snapshot.FetchSnapshotByVersionHref(
		cps.ctx,
		cps.repoConfig.UUID,
		versionHref,
	)
	if err != nil {
		return fmt.Errorf("failed to check for existing snapshot: %w", err)
	}
	if existingSnap != nil {
		cps.logger.Debug().
			Str("snapshot_uuid", existingSnap.UUID).
			Str("version_href", versionHref).
			Msg("Snapshot already exists for this version, skipping creation")
		return nil // Success - idempotent
	}

	helper := SnapshotHelper{
		pulpClient: cps.pulpClient,
		ctx:        cps.ctx,
		payload:    cps,
		logger:     cps.logger,
		orgId:      cps.orgID,
		repo:       cps.repoConfig,
		daoReg:     cps.daoReg,
		domainName: cps.domainName,
	}

	err = helper.Run(versionHref)
	if err != nil {
		return fmt.Errorf("failed to create snapshot: %w", err)
	}

	// Update snapshot with Copr build ID
	if cps.payload.SnapshotUUID != nil && cps.payload.CoprBuildID != "" {
		err = cps.daoReg.Snapshot.InternalOnly_UpdateCoprBuildID(
			cps.ctx,
			*cps.payload.SnapshotUUID,
			cps.payload.CoprBuildID,
		)
		if err != nil {
			return fmt.Errorf("failed to update snapshot with Copr build ID: %w", err)
		}
	}

	return nil
}

func (cps *CreatePartnerSnapshot) findOrCreateRepoConfig() (api.RepositoryResponse, error) {
	repoConfig, err := cps.daoReg.RepositoryConfig.InternalOnly_FetchRepoConfigByCoprRepositoryHref(
		cps.ctx,
		cps.orgID,
		cps.payload.CoprRepositoryHref,
	)
	if err == nil {
		cps.logger.Debug().
			Str("repo_config_uuid", repoConfig.UUID).
			Str("copr_repository_href", cps.payload.CoprRepositoryHref).
			Msg("Found existing repository configuration by Copr repository href")
		return repoConfig, nil
	}

	var daoErr *ce.DaoError
	if !errors.As(err, &daoErr) || !daoErr.NotFound {
		return api.RepositoryResponse{}, err
	}

	cps.logger.Debug().
		Str("repository_name", cps.payload.RepositoryName).
		Str("version", cps.payload.RepositoryVersion).
		Str("arch", cps.payload.RepositoryArch).
		Str("copr_project_id", cps.payload.CoprProjectID).
		Str("copr_repository_href", cps.payload.CoprRepositoryHref).
		Msg("Creating new repository configuration")

	newRepoReq := api.RepositoryRequest{
		Name:                 utils.Ptr(cps.payload.RepositoryName),
		DistributionVersions: utils.Ptr([]string{cps.payload.RepositoryVersion}),
		DistributionArch:     utils.Ptr(cps.payload.RepositoryArch),
		OrgID:                utils.Ptr(cps.orgID),
		AccountID:            utils.Ptr(cps.accountID),
		Origin:               utils.Ptr(config.OriginUpload),
		Snapshot:             utils.Ptr(true),
	}

	repoConfig, err = cps.daoReg.RepositoryConfig.Create(cps.ctx, newRepoReq)
	if err != nil {
		return api.RepositoryResponse{}, fmt.Errorf("failed to create repository config: %w", err)
	}

	err = cps.daoReg.RepositoryConfig.InternalOnly_UpdateCoprMetadata(
		cps.ctx,
		repoConfig.UUID,
		cps.payload.CoprProjectID,
		cps.payload.CoprRepositoryHref,
	)
	if err != nil {
		return api.RepositoryResponse{}, fmt.Errorf("failed to update Copr metadata: %w", err)
	}

	cps.logger.Info().
		Str("repo_config_uuid", repoConfig.UUID).
		Str("repository_uuid", repoConfig.RepositoryUUID).
		Str("copr_repository_href", cps.payload.CoprRepositoryHref).
		Msg("Created new repository configuration with Copr metadata")

	return repoConfig, nil
}

// ResumableSnapshotInterface implementation

func (cps *CreatePartnerSnapshot) SavePublicationTaskHref(href string) error {
	// No-op - publication already exists (created by Copr)
	return nil
}

func (cps *CreatePartnerSnapshot) GetPublicationTaskHref() *string {
	return nil // Publication created by Copr, not by us
}

func (cps *CreatePartnerSnapshot) SaveDistributionTaskHref(href string) error {
	if href == "" {
		cps.payload.DistributionTaskHref = nil
	} else {
		cps.payload.DistributionTaskHref = &href
	}
	return cps.UpdatePayload()
}

func (cps *CreatePartnerSnapshot) GetDistributionTaskHref() *string {
	return cps.payload.DistributionTaskHref
}

func (cps *CreatePartnerSnapshot) SaveSnapshotIdent(id string) error {
	if id == "" {
		cps.payload.SnapshotIdent = nil
	} else {
		cps.payload.SnapshotIdent = &id
	}
	return cps.UpdatePayload()
}

func (cps *CreatePartnerSnapshot) GetSnapshotIdent() *string {
	return cps.payload.SnapshotIdent
}

func (cps *CreatePartnerSnapshot) SaveSnapshotUUID(uuid string) error {
	if uuid == "" {
		cps.payload.SnapshotUUID = nil
	} else {
		cps.payload.SnapshotUUID = &uuid
	}
	return cps.UpdatePayload()
}

func (cps *CreatePartnerSnapshot) GetSnapshotUUID() *string {
	return cps.payload.SnapshotUUID
}

func (cps *CreatePartnerSnapshot) UpdatePayload() error {
	var err error
	a := *cps.payload
	cps.task, err = (*cps.queue).UpdatePayload(cps.task, a)
	return err
}
