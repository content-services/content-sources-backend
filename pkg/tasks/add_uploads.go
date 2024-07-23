package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/candlepin_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	"github.com/content-services/yummy/pkg/yum"
	"github.com/openlyinc/pointy"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type AddUploadsPayload struct {
	RepositoryConfigUUID string
	Artifacts            []api.Artifact
	Uploads              []api.Upload

	VersionHref          *string // href of repo version created as part of this
	PublicationTaskHref  *string //
	DistributionTaskHref *string
	SnapshotIdent        *string
}

type AddUploads struct {
	orgID      string
	domainName string
	ctx        context.Context
	payload    *AddUploadsPayload
	task       *models.TaskInfo
	daoReg     *dao.DaoRegistry
	repo       api.RepositoryResponse
	cpClient   candlepin_client.CandlepinClient
	pulpClient pulp_client.PulpClient
	queue      *queue.Queue
	logger     *zerolog.Logger
}

func AddUploadsHandler(ctx context.Context, task *models.TaskInfo, queue *queue.Queue) error {
	if config.Get().Clients.Candlepin.Server == "" {
		return nil
	}

	opts := AddUploadsPayload{}
	if err := json.Unmarshal(task.Payload, &opts); err != nil {
		return fmt.Errorf("payload incorrect type for " + config.AddUploadsTask)
	}

	logger := LogForTask(task.Id.String(), task.Typename, task.RequestID)
	ctxWithLogger := logger.WithContext(ctx)

	daoReg := dao.GetDaoRegistry(db.DB)
	domainName, err := daoReg.Domain.Fetch(ctxWithLogger, task.OrgId)
	if err != nil {
		return err
	}

	pulpClient := pulp_client.GetPulpClientWithDomain(domainName)
	cpClient := candlepin_client.NewCandlepinClient()
	repo, err := daoReg.RepositoryConfig.Fetch(ctx, task.OrgId, opts.RepositoryConfigUUID)
	if err != nil {
		return fmt.Errorf("could not fetch repository config %w", err)
	}

	ur := AddUploads{
		daoReg:     daoReg,
		payload:    &opts,
		task:       task,
		ctx:        ctx,
		domainName: domainName,
		cpClient:   cpClient,
		orgID:      task.OrgId,
		repo:       repo,
		pulpClient: pulpClient,
		queue:      queue,
		logger:     logger,
	}
	return ur.Run()
}

func (ur *AddUploads) Run() (err error) {
	if ur.payload.VersionHref == nil {
		artifacts, err := ur.ConvertUploadsToArtifacts()
		if err != nil {
			return fmt.Errorf("could not convert uploads to artifacts %w", err)
		}

		contentHrefs, err := ur.ConvertArtifactsToPackages(artifacts)
		if err != nil {
			return fmt.Errorf("could not convert artifacts to contentHrefs %w", err)
		}

		versionHref, err := ur.AddContentToRepo(contentHrefs)
		if err != nil {
			return err
		}
		if versionHref == "" {
			log.Debug().Msgf("added content to repository %v, but no new version created, likely already present", ur.payload.RepositoryConfigUUID)
			return nil
		}
		ur.payload.VersionHref = &versionHref
		err = ur.UpdatePayload()
		if err != nil {
			return fmt.Errorf("could not update payload %w", err)
		}
	}

	helper := SnapshotHelper{
		pulpClient: ur.pulpClient,
		ctx:        ur.ctx,
		payload:    ur,
		logger:     ur.logger,
		orgId:      ur.orgID,
		repo:       ur.repo,
		daoReg:     ur.daoReg,
		domainName: ur.domainName,
	}
	defer func() {
		if errors.Is(err, context.Canceled) {
			cleanupErr := helper.Cleanup()
			if cleanupErr != nil {
				ur.logger.Err(cleanupErr).Msg("error cleaning up canceled snapshot helper")
			}
		}
	}()

	err = helper.Run(*ur.payload.VersionHref)
	if err != nil {
		return err
	}

	err = ur.ImportPackageData(*ur.payload.VersionHref)
	if err != nil {
		return fmt.Errorf("could not import package data %w", err)
	}
	return nil
}

func (ur *AddUploads) AddContentToRepo(contentHrefs []string) (versionHref string, err error) {
	repo, err := ur.pulpClient.GetRpmRepositoryByName(ur.ctx, ur.repo.UUID)
	if err != nil {
		return "", fmt.Errorf("could not get repository info %w", err)
	}

	// Modify Repository Content
	task, err := ur.pulpClient.ModifyRpmRepositoryContent(ur.ctx, *repo.PulpHref, contentHrefs, []string{})
	if err != nil {
		return "", fmt.Errorf("could not modify repository contents %w", err)
	}
	result, err := ur.pulpClient.PollTask(ur.ctx, task)
	if err != nil {
		return "", fmt.Errorf("modify repo task failed %w", err)
	}

	if len(result.CreatedResources) == 0 {
		// RPMs must have already been in the repository
		return "", nil
	} else if len(result.CreatedResources) > 2 {
		return "", fmt.Errorf("unexpectedly got more than 1 Created resources after ModifyRpmRepositoryContent: %v", result.CreatedResources)
	} else {
		return result.CreatedResources[0], nil
	}
}

type pendingArtifact struct {
	Upload   api.Upload
	TaskHref string
}

func (ur *AddUploads) ConvertUploadsToArtifacts() ([]api.Artifact, error) {
	artifacts := ur.payload.Artifacts
	var pendingArtifacts []pendingArtifact
	// Convert any uploads to artifacts, or lookup uploads that are already artifacts
	for _, upload := range ur.payload.Uploads {
		found, err := ur.pulpClient.LookupArtifact(ur.ctx, upload.Sha256)
		if err != nil {
			return artifacts, fmt.Errorf("could not lookup artifact: %w", err)
		}
		if found == nil {
			task, err := ur.pulpClient.FinishUpload(ur.ctx, upload.Href, upload.Sha256)
			if err != nil {
				return artifacts, fmt.Errorf("could not finish upload: %w", err)
			}
			if task != nil {
				pendingArtifacts = append(pendingArtifacts, pendingArtifact{
					Upload:   upload,
					TaskHref: task.Task,
				})
			}
		} else {
			artifacts = append(artifacts, api.Artifact{Sha256: upload.Sha256, Href: *found})
			continue
		}
	}

	for _, pendArt := range pendingArtifacts {
		result, err := ur.pulpClient.PollTask(ur.ctx, pendArt.TaskHref)
		if err != nil {
			return artifacts, fmt.Errorf("finish upload task failed unexpectedly : %w", err)
		}
		if len(result.CreatedResources) != 1 {
			return artifacts, fmt.Errorf("expected one created resource but got %d", len(result.CreatedResources))
		}
		artifacts = append(artifacts, api.Artifact{Sha256: pendArt.Upload.Sha256, Href: result.CreatedResources[0]})
	}

	return artifacts, nil
}

type pendingPackage struct {
	Artifact api.Artifact
	TaskHref string
}

func (ur *AddUploads) ConvertArtifactsToPackages(artifacts []api.Artifact) (contentHrefs []string, err error) {
	contentHrefs = []string{}
	pendingPackages := []pendingPackage{}

	for _, artifact := range artifacts {
		pkg, err := ur.pulpClient.LookupPackage(ur.ctx, artifact.Sha256)
		if err != nil {
			return contentHrefs, fmt.Errorf("Could not lookup package %w", err)
		}
		if pkg == nil {
			// package doesn't exist, so convert the artifact to a package
			task, err := ur.pulpClient.CreatePackage(ur.ctx, &artifact.Href, nil)
			if err != nil {
				return contentHrefs, fmt.Errorf("could not create package %w", err)
			}
			pendingPackages = append(pendingPackages, pendingPackage{
				Artifact: artifact,
				TaskHref: task,
			})
		} else {
			contentHrefs = append(contentHrefs, *pkg)
		}
	}

	for _, pendArt := range pendingPackages {
		result, err := ur.pulpClient.PollTask(ur.ctx, pendArt.TaskHref)
		if err != nil {
			return contentHrefs, fmt.Errorf("finish upload task failed unexpectedly : %w", err)
		}
		if len(result.CreatedResources) != 1 {
			return contentHrefs, fmt.Errorf("expected one created resource but got %d", len(result.CreatedResources))
		}
		contentHrefs = append(contentHrefs, result.CreatedResources[0])
	}

	return contentHrefs, err
}

func (ur *AddUploads) UpdatePayload() error {
	var err error
	a := *ur.payload
	ur.task, err = (*ur.queue).UpdatePayload(ur.task, a)
	if err != nil {
		return err
	}
	return nil
}

func (ur *AddUploads) SavePublicationTaskHref(href string) error {
	ur.payload.PublicationTaskHref = &href
	return ur.UpdatePayload()
}

func (ur *AddUploads) GetPublicationTaskHref() *string {
	return ur.payload.PublicationTaskHref
}

func (ur *AddUploads) SaveDistributionTaskHref(href string) error {
	ur.payload.DistributionTaskHref = &href
	return ur.UpdatePayload()
}

func (ur *AddUploads) GetDistributionTaskHref() *string {
	return ur.payload.DistributionTaskHref
}

func (ur *AddUploads) SaveSnapshotIdent(id string) error {
	ur.payload.SnapshotIdent = &id
	return ur.UpdatePayload()
}

func (ur *AddUploads) GetSnapshotIdent() *string {
	return ur.payload.SnapshotIdent
}

func (ur *AddUploads) ImportPackageData(versionHref string) error {
	pkgs, err := ur.pulpClient.ListVersionAllPackages(ur.ctx, versionHref)
	if err != nil {
		return fmt.Errorf("could not list all packages: %w", err)
	}
	// convert from tangy to yummy format
	yumPkgs := []yum.Package{}
	for _, rpm := range pkgs {
		epoch := 0
		if rpm.Epoch != nil {
			epoch, err = strconv.Atoi(*rpm.Epoch)
			if err != nil {
				epoch = 0
			}
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
				Epoch:   int32(epoch),
			},
			Checksum: yum.Checksum{
				Value: *rpm.Sha256,
			},
			Summary: *rpm.Summary,
		})
	}
	_, err = ur.daoReg.Rpm.InsertForRepository(ur.ctx, ur.repo.RepositoryUUID, yumPkgs)
	if err != nil {
		return fmt.Errorf("could not insert packages: %w", err)
	}
	currentTime := time.Now()
	err = ur.daoReg.Repository.Update(ur.ctx, dao.RepositoryUpdate{
		UUID:                         ur.repo.RepositoryUUID,
		LastIntrospectionTime:        pointy.Pointer(currentTime),
		LastIntrospectionSuccessTime: pointy.Pointer(currentTime),
		LastIntrospectionUpdateTime:  pointy.Pointer(currentTime),
		LastIntrospectionError:       nil,
		LastIntrospectionStatus:      pointy.Pointer(config.StatusValid),
		PackageCount:                 pointy.Pointer(len(yumPkgs)),
		FailedIntrospectionsCount:    pointy.Pointer(0),
	})
	return err
}
