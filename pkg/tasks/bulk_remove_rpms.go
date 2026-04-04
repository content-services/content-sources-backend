package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	"github.com/content-services/content-sources-backend/pkg/utils"
	"github.com/content-services/yummy/pkg/yum"
	"github.com/rs/zerolog"
)

type BulkRemoveRpmsPayload struct {
	RepositoryConfigUUID string
	RpmUuids             []string

	VersionHref          *string
	PublicationTaskHref  *string
	DistributionTaskHref *string
	SnapshotIdent        *string
	SnapshotUUID         *string
}

type BulkRemoveRpms struct {
	orgID      string
	domainName string
	ctx        context.Context
	payload    *BulkRemoveRpmsPayload
	task       *models.TaskInfo
	daoReg     *dao.DaoRegistry
	repo       api.RepositoryResponse
	pulpClient pulp_client.PulpClient
	queue      *queue.Queue
	logger     *zerolog.Logger
}

func BulkRemoveRpmsHandler(ctx context.Context, task *models.TaskInfo, queue *queue.Queue) error {
	if !config.PulpConfigured() {
		return nil
	}

	opts := BulkRemoveRpmsPayload{}
	if err := json.Unmarshal(task.Payload, &opts); err != nil {
		return fmt.Errorf("payload incorrect type for " + config.BulkRemoveRpmsTask)
	}

	logger := LogForTask(task.Id.String(), task.Typename, task.RequestID)
	ctxWithLogger := logger.WithContext(ctx)

	daoReg := dao.GetDaoRegistry(db.DB)

	repo, err := daoReg.RepositoryConfig.Fetch(ctx, task.OrgId, opts.RepositoryConfigUUID)
	if err != nil {
		return fmt.Errorf("could not fetch repository config %w", err)
	}

	domainName, err := daoReg.Domain.Fetch(ctxWithLogger, task.OrgId)
	if err != nil {
		return err
	}
	pulpClient := pulp_client.GetPulpClientWithDomain(domainName)

	r := BulkRemoveRpms{
		daoReg:     daoReg,
		payload:    &opts,
		task:       task,
		ctx:        ctx,
		domainName: domainName,
		orgID:      task.OrgId,
		repo:       repo,
		pulpClient: pulpClient,
		queue:      queue,
		logger:     logger,
	}
	return r.Run()
}

func (r *BulkRemoveRpms) Run() (err error) {
	if r.payload.VersionHref == nil {
		versionHref, err := r.removeContentFromRepo()
		if err != nil {
			return err
		}
		r.payload.VersionHref = &versionHref
		err = r.UpdatePayload()
		if err != nil {
			return fmt.Errorf("could not update payload %w", err)
		}
	}

	helper := SnapshotHelper{
		pulpClient: r.pulpClient,
		ctx:        r.ctx,
		payload:    r,
		logger:     r.logger,
		orgId:      r.orgID,
		repo:       r.repo,
		daoReg:     r.daoReg,
		domainName: r.domainName,
	}
	defer func() {
		if errors.Is(err, context.Canceled) {
			cleanupErr := helper.Cleanup()
			if cleanupErr != nil {
				r.logger.Err(cleanupErr).Msg("error cleaning up canceled snapshot helper")
			}
		}
	}()

	err = helper.Run(*r.payload.VersionHref)
	if err != nil {
		return err
	}

	err = r.ImportPackageData(*r.payload.VersionHref)
	if err != nil {
		return fmt.Errorf("could not import package data %w", err)
	}
	return nil
}

func (r *BulkRemoveRpms) removeContentFromRepo() (versionHref string, err error) {
	rpms, err := r.daoReg.Rpm.FetchForRepository(r.ctx, r.orgID, r.payload.RepositoryConfigUUID, r.payload.RpmUuids)
	if err != nil {
		return "", fmt.Errorf("could not fetch RPMs: %w", err)
	}

	var contentHrefs []string
	for _, rpm := range rpms {
		sha := rpmChecksumHexForPulp(rpm.Checksum)
		href, err := r.pulpClient.LookupPackage(r.ctx, sha)
		if err != nil {
			return "", fmt.Errorf("could not lookup package in pulp: %w", err)
		}
		if href == nil {
			return "", fmt.Errorf("package for RPM %s not found in pulp", rpm.UUID)
		}
		contentHrefs = append(contentHrefs, *href)
	}

	repo, err := r.pulpClient.GetRpmRepositoryByName(r.ctx, r.repo.UUID)
	if err != nil {
		return "", fmt.Errorf("could not get repository %w", err)
	}

	task, err := r.pulpClient.ModifyRpmRepositoryContent(r.ctx, *repo.PulpHref, []string{}, contentHrefs)
	if err != nil {
		return "", fmt.Errorf("could not modify repository contents %w", err)
	}
	result, err := r.pulpClient.PollTask(r.ctx, task)
	if err != nil {
		return "", fmt.Errorf("modify repo task failed %w", err)
	}

	if len(result.CreatedResources) == 0 {
		return "", nil
	}
	if len(result.CreatedResources) > 2 {
		return "", fmt.Errorf("unexpectedly got more than 1 Created resource after ModifyRpmRepositoryContent: %v", result.CreatedResources)
	}
	return result.CreatedResources[0], nil
}

func rpmChecksumHexForPulp(checksum string) string {
	c := strings.TrimPrefix(checksum, "SHA256:")
	return strings.TrimPrefix(c, "sha256:")
}

func (r *BulkRemoveRpms) UpdatePayload() error {
	var err error
	a := *r.payload
	r.task, err = (*r.queue).UpdatePayload(r.task, a)
	if err != nil {
		return err
	}
	return nil
}

func (r *BulkRemoveRpms) ImportPackageData(versionHref string) error {
	pkgs, err := r.pulpClient.ListVersionAllPackages(r.ctx, versionHref)
	if err != nil {
		return fmt.Errorf("could not list all packages: %w", err)
	}
	yumPkgs := []yum.Package{}
	for _, rpm := range pkgs {
		epoch := int32(0)
		if rpm.Epoch != nil {
			epochConv, err := strconv.ParseInt(*rpm.Epoch, 10, 32)
			if err != nil {
				return err
			}
			if epochConv < math.MinInt32 || epochConv > math.MaxInt32 {
				return fmt.Errorf("invalid epoch %d", epoch)
			}
			epoch = int32(epochConv)
		}

		if rpm.Name == nil || rpm.Arch == nil || rpm.Version == nil || rpm.Release == nil || rpm.Sha256 == nil || rpm.Summary == nil {
			return fmt.Errorf("received nil values for rpm packages in version %v from pulp", versionHref)
		}
		yumPkgs = append(yumPkgs, yum.Package{
			Name: *rpm.Name,
			Arch: *rpm.Arch,
			Version: yum.Version{
				Version: *rpm.Version,
				Release: *rpm.Release,
				Epoch:   epoch,
			},
			Checksum: yum.Checksum{
				Value: *rpm.Sha256,
			},
			Summary: *rpm.Summary,
		})
	}
	_, err = r.daoReg.Rpm.InsertForRepository(r.ctx, r.repo.RepositoryUUID, yumPkgs)
	if err != nil {
		return fmt.Errorf("could not insert packages: %w", err)
	}
	currentTime := time.Now()
	err = r.daoReg.Repository.Update(r.ctx, dao.RepositoryUpdate{
		UUID:                         r.repo.RepositoryUUID,
		LastIntrospectionTime:        utils.Ptr(currentTime),
		LastIntrospectionSuccessTime: utils.Ptr(currentTime),
		LastIntrospectionUpdateTime:  utils.Ptr(currentTime),
		LastIntrospectionError:       nil,
		LastIntrospectionStatus:      utils.Ptr(config.StatusValid),
		PackageCount:                 utils.Ptr(len(yumPkgs)),
		FailedIntrospectionsCount:    utils.Ptr(0),
	})
	return err
}

func (r *BulkRemoveRpms) GetDistributionTaskHref() *string {
	return r.payload.DistributionTaskHref
}

func (r *BulkRemoveRpms) GetPublicationTaskHref() *string {
	return r.payload.PublicationTaskHref
}

func (r *BulkRemoveRpms) GetSnapshotIdent() *string {
	return r.payload.SnapshotIdent
}

func (r *BulkRemoveRpms) GetSnapshotUUID() *string {
	return r.payload.SnapshotUUID
}

func (r *BulkRemoveRpms) SaveDistributionTaskHref(href string) error {
	r.payload.DistributionTaskHref = &href
	return r.UpdatePayload()
}

func (r *BulkRemoveRpms) SavePublicationTaskHref(href string) error {
	r.payload.PublicationTaskHref = &href
	return r.UpdatePayload()
}

func (r *BulkRemoveRpms) SaveSnapshotIdent(id string) error {
	r.payload.SnapshotIdent = &id
	return r.UpdatePayload()
}

func (r *BulkRemoveRpms) SaveSnapshotUUID(uuid string) error {
	r.payload.SnapshotUUID = &uuid
	return r.UpdatePayload()
}
