package seeds

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"strconv"
	"time"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/utils"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type SeedOptions struct {
	OrgID       string
	BatchSize   int
	Arch        *string
	Versions    *[]string
	Status      *string
	ContentType *string
	Origin      *string
	Version     *string
	TaskID      string
}

type IntrospectionStatusMetadata struct {
	LastIntrospectionStatus      *string
	lastIntrospectionTime        *time.Time
	lastIntrospectionSuccessTime *time.Time
	lastIntrospectionUpdateTime  *time.Time
	lastIntrospectionError       *string
}

const (
	batchSize = 500
)

func randomURL() string {
	return fmt.Sprintf("https://%s.com/%s", RandStringBytes(20), RandStringBytes(5))
}

func randomRepositoryRpmName() string {
	return RandStringBytes(12)
}

var (
	archs = []string{
		"x86_64",
		"noarch",
	}
)

func randomRepositoryRpmArch() string {
	return archs[rand.Int()%len(archs)]
}

func SeedRepositoryConfigurations(db *gorm.DB, size int, options SeedOptions) ([]models.RepositoryConfiguration, error) {
	var repos []models.Repository
	var repoConfigurations []models.RepositoryConfiguration

	if options.BatchSize != 0 {
		db.CreateBatchSize = options.BatchSize
	}
	if options.ContentType == nil {
		options.ContentType = utils.Ptr(config.ContentTypeRpm)
	}
	if options.Origin == nil {
		options.Origin = utils.Ptr(config.OriginExternal)
	}

	for i := 0; i < size; i++ {
		introspectionMetadata := randomIntrospectionStatusMetadata(options.Status)
		repo := models.Repository{
			URL:                          randomURL(),
			LastIntrospectionTime:        introspectionMetadata.lastIntrospectionTime,
			LastIntrospectionSuccessTime: introspectionMetadata.lastIntrospectionSuccessTime,
			LastIntrospectionUpdateTime:  introspectionMetadata.lastIntrospectionUpdateTime,
			LastIntrospectionError:       introspectionMetadata.lastIntrospectionError,
			LastIntrospectionStatus:      *introspectionMetadata.LastIntrospectionStatus,
			Origin:                       *options.Origin,
			ContentType:                  *options.ContentType,
		}
		repos = append(repos, repo)
	}
	if err := db.Create(&repos).Error; err != nil {
		return nil, err
	}

	for i := 0; i < size; i++ {
		repoConfig := models.RepositoryConfiguration{
			Name:                 fmt.Sprintf("%s - %s - %s", RandStringBytes(2), "TestRepo", RandStringBytes(10)),
			Label:                fmt.Sprintf("%s - %s - %s", RandStringBytes(2), "TestRepo", RandStringBytes(10)),
			Versions:             createVersionArray(options.Versions),
			Arch:                 createArch(options.Arch),
			AccountID:            fmt.Sprintf("%d", rand.Intn(9999)),
			OrgID:                createOrgId(options.OrgID),
			RepositoryUUID:       repos[i].UUID,
			LastSnapshotTaskUUID: options.TaskID,
			Snapshot:             true,
		}
		repoConfigurations = append(repoConfigurations, repoConfig)
	}
	if err := db.Create(&repoConfigurations).Error; err != nil {
		return nil, fmt.Errorf("could not save seed: %w", err)
	}
	return repoConfigurations, nil
}

func randomIntrospectionStatusMetadata(existingStatus *string) IntrospectionStatusMetadata {
	var metadata IntrospectionStatusMetadata

	if existingStatus != nil {
		metadata = getIntrospectionTimestamps(*existingStatus)
		return metadata
	}

	statuses := []string{config.StatusPending, config.StatusValid, config.StatusInvalid, config.StatusUnavailable}
	index := rand.Intn(4)

	return getIntrospectionTimestamps(statuses[index])
}

func getIntrospectionTimestamps(lastIntrospectionStatus string) IntrospectionStatusMetadata {
	timestamp := time.Now()
	metadata := IntrospectionStatusMetadata{LastIntrospectionStatus: &lastIntrospectionStatus}

	switch lastIntrospectionStatus {
	case config.StatusValid:
		metadata.lastIntrospectionTime = &timestamp
		metadata.lastIntrospectionSuccessTime = &timestamp
		metadata.lastIntrospectionUpdateTime = &timestamp
	case config.StatusInvalid:
		metadata.lastIntrospectionError = utils.Ptr("bad introspection")
	case config.StatusUnavailable:
		metadata.lastIntrospectionTime = &timestamp
		metadata.lastIntrospectionSuccessTime = &timestamp
		metadata.lastIntrospectionUpdateTime = &timestamp
		metadata.lastIntrospectionError = utils.Ptr("bad introspection")
	}
	return metadata
}

func SeedRepository(db *gorm.DB, size int, options SeedOptions) error {
	var repos []models.Repository

	// Add size random Repository entries
	countRecords := 0
	for i := 0; i < size; i++ {
		introspectionMetadata := randomIntrospectionStatusMetadata(options.Status)

		repo := models.Repository{
			URL:                          randomURL(),
			LastIntrospectionTime:        introspectionMetadata.lastIntrospectionTime,
			LastIntrospectionSuccessTime: introspectionMetadata.lastIntrospectionSuccessTime,
			LastIntrospectionUpdateTime:  introspectionMetadata.lastIntrospectionUpdateTime,
			LastIntrospectionError:       introspectionMetadata.lastIntrospectionError,
			LastIntrospectionStatus:      *introspectionMetadata.LastIntrospectionStatus,
			Public:                       true,
		}
		repos = append(repos, repo)
		if len(repos) >= batchSize {
			if r := db.Create(repos); r != nil && r.Error != nil {
				return r.Error
			}
			countRecords += len(repos)
			repos = []models.Repository{}
			fmt.Printf("repoConfig: %d        \r", countRecords)
		}
	}

	// Add remaining records
	if len(repos) > 0 {
		r := db.Create(&repos)
		if r.Error != nil {
			return r.Error
		}
		countRecords += len(repos)
		fmt.Printf("repoConfig: %d        \r", countRecords)
	}

	return nil
}

func SeedSnapshots(db *gorm.DB, repoConfigUuid string, size int) ([]models.Snapshot, error) {
	created := []models.Snapshot{}
	for i := 0; i < size; i++ {
		path := fmt.Sprintf("/seed/%v/%v", repoConfigUuid, i)
		snap := models.Snapshot{
			VersionHref:                 path,
			PublicationHref:             path,
			DistributionPath:            path,
			DistributionHref:            path,
			RepositoryConfigurationUUID: repoConfigUuid,
			ContentCounts:               models.ContentCountsType{},
			AddedCounts:                 models.ContentCountsType{},
			RemovedCounts:               models.ContentCountsType{},
		}
		res := db.Create(&snap)
		created = append(created, snap)
		if res.Error != nil {
			return nil, res.Error
		}
	}
	return created, nil
}

type TemplateSeedOptions struct {
	OrgID                 string
	BatchSize             int
	Arch                  *string
	Version               *string
	RepositoryConfigUUIDs []string
}

func SeedTemplates(db *gorm.DB, size int, options TemplateSeedOptions) ([]models.Template, error) {
	orgID := RandomOrgId()
	templates := []models.Template{}
	if options.OrgID != "" {
		orgID = options.OrgID
	}
	for i := 0; i < size; i++ {
		t := models.Template{
			Base: models.Base{
				UUID: uuid.NewString(),
			},
			Name:          RandStringBytes(10),
			OrgID:         orgID,
			Description:   "description",
			Date:          time.Now(),
			Version:       createVersion(options.Version),
			Arch:          createArch(options.Arch),
			CreatedBy:     "user",
			LastUpdatedBy: "user",
		}
		err := db.Create(&t).Error
		if err != nil {
			return nil, err
		}
		var tRepos []models.TemplateRepositoryConfiguration
		for _, rcUUID := range options.RepositoryConfigUUIDs {
			tRepos = append(tRepos, models.TemplateRepositoryConfiguration{
				RepositoryConfigurationUUID: rcUUID,
				TemplateUUID:                t.UUID,
			})
		}
		err = db.Create(&tRepos).Error
		templates = append(templates, t)
		if err != nil {
			return nil, err
		}
	}
	return templates, nil
}

// SeedRpms Populate database with random package information
// db The database descriptor.
// size The number of rpm packages per repository to be generated.
func SeedRpms(db *gorm.DB, repo *models.Repository, size int) error {
	if db == nil {
		return fmt.Errorf("db cannot be nil")
	}
	if repo == nil {
		return fmt.Errorf("repo cannot be nil")
	}
	if size < 0 {
		return fmt.Errorf("size cannot be lower than 0")
	}
	if size == 0 {
		return nil
	}
	var rpms []models.Rpm

	var repositories_rpms []map[string]interface{}

	// For each repo add 'size' rpm random packages
	for i := 0; i < size; i++ {
		rpm := models.Rpm{
			Name:     randomRepositoryRpmName(),
			Arch:     randomRepositoryRpmArch(),
			Version:  fmt.Sprintf("%d.%d.%d", rand.Int()%6, rand.Int()%16, rand.Int()%64),
			Release:  fmt.Sprintf("%d", rand.Int()%128),
			Epoch:    0,
			Summary:  "Package summary",
			Checksum: RandStringBytes(64),
		}
		rpms = append(rpms, rpm)
	}

	if err := db.Create(rpms).Error; err != nil {
		return err
	}

	for _, rpm := range rpms {
		repositories_rpms = append(repositories_rpms, map[string]interface{}{
			"repository_uuid": repo.Base.UUID,
			"rpm_uuid":        rpm.Base.UUID,
		})
	}
	if err := db.Table(models.TableNameRpmsRepositories).Create(&repositories_rpms).Error; err != nil {
		return err
	}
	return nil
}

func RandomOrgId() string {
	return strconv.Itoa(rand.Intn(99999999))
}

func RandomAccountId() string {
	return strconv.Itoa(rand.Intn(99999999))
}

// createOrgId aims to mainly create most entities in the same org
// but also create some in other random orgs
func createOrgId(existingOrgId string) string {
	orgId := "4234"
	if existingOrgId != "" {
		orgId = existingOrgId
	} else {
		randomNum := rand.Intn(5)
		if randomNum == 3 {
			orgId = RandomOrgId()
		}
	}
	return orgId
}

func createVersionArray(existingVersionArray *[]string) []string {
	if existingVersionArray != nil && len(*existingVersionArray) != 0 {
		return *existingVersionArray
	}
	versionArray := make([]string, 0)
	distVersLength := len(config.DistributionVersions)
	for k := rand.Intn(distVersLength - 1); k < distVersLength; k++ {
		ver := config.DistributionVersions[k].Label
		if ver != config.ANY_VERSION {
			versionArray = append(versionArray, ver)
		}
	}

	return versionArray
}

func createVersion(existingVersion *string) string {
	version := config.El9
	if existingVersion != nil && *existingVersion != "" {
		version = *existingVersion
		return version
	}
	randomNum := rand.Intn(20)
	if randomNum < 4 {
		version = config.El8
	}
	if randomNum > 4 && randomNum < 6 {
		version = config.El7
	}
	return version
}

func createArch(existingArch *string) string {
	arch := config.X8664
	if existingArch != nil && *existingArch != "" {
		arch = *existingArch
		return arch
	}
	randomNum := rand.Intn(20)
	if randomNum < 4 {
		arch = config.ANY_ARCH
	}
	if randomNum > 4 && randomNum < 6 {
		arch = config.S390x
	}
	return arch
}

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

// RandStringWithChars Return a random string of size n using the lookup
// table.
// n size of the string to be returned
// lookup A string representing the lookup table.
// Return the random string.
func RandStringWithChars(n int, lookup string) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = lookup[rand.Intn(len(lookup))]
	}
	return string(b)
}

func RandStringBytes(n int) string {
	return RandStringWithChars(n, letterBytes)
}

type TaskSeedOptions struct {
	AccountID      string
	OrgID          string
	BatchSize      int
	Status         string
	Error          *string
	Typename       string
	RepoConfigUUID string
	RepoUUID       string
	QueuedAt       *time.Time
	FinishedAt     *time.Time
}

func SeedTasks(db *gorm.DB, size int, options TaskSeedOptions) ([]models.TaskInfo, error) {
	var err error
	var tasks []models.TaskInfo
	var repoUUID string
	if options.BatchSize != 0 {
		db.CreateBatchSize = options.BatchSize
	}

	orgId := createOrgId(options.OrgID)

	if options.RepoUUID == "" {
		repo := models.Repository{
			URL: randomURL(),
		}
		if err := db.Create(&repo).Error; err != nil {
			return tasks, err
		}
		repoUUID = repo.UUID
	} else {
		repoUUID = options.RepoUUID
	}

	if options.AccountID != "" || options.RepoConfigUUID != "" {
		var repoConfig models.RepositoryConfiguration
		if options.RepoConfigUUID != "" {
			err = db.Where("uuid = ? ", options.RepoConfigUUID).First(&repoConfig).Error
		} else {
			repoConfig = models.RepositoryConfiguration{
				RepositoryUUID: repoUUID,
				AccountID:      options.AccountID,
				Name:           fmt.Sprintf("%s - %s - %s", RandStringBytes(2), "TestRepo", RandStringBytes(10)),
				OrgID:          orgId,
			}
			err = db.Create(&repoConfig).Error
		}
		repoUUID = repoConfig.RepositoryUUID
		orgId = createOrgId(repoConfig.OrgID)
		if err != nil {
			return tasks, err
		}
	}

	typename := "example type"
	if options.Typename != "" {
		typename = options.Typename
	}

	payloadData := map[string]string{"url": "https://example.com"}
	payload, err := json.Marshal(payloadData)
	if err != nil {
		return tasks, err
	}

	tasks = make([]models.TaskInfo, size)
	repoUUIDParsed := uuid.MustParse(repoUUID)

	for i := 0; i < size; i++ {
		queued := time.Now().Add(time.Minute * time.Duration(i))
		if options.QueuedAt != nil {
			queued = *options.QueuedAt
		}
		started := time.Now().Add(time.Minute * time.Duration(i+5))
		finished := time.Now().Add(time.Minute * time.Duration(i+10))
		if options.FinishedAt != nil {
			started = (*options.FinishedAt).Add(-5 * time.Minute)
			finished = *options.FinishedAt
		}
		tasks[i] = models.TaskInfo{
			Id:           uuid.New(),
			Typename:     typename,
			Payload:      payload,
			OrgId:        orgId,
			AccountId:    options.AccountID,
			ObjectUUID:   repoUUIDParsed,
			ObjectType:   utils.Ptr(config.ObjectTypeRepository),
			Dependencies: make([]string, 0),
			Token:        uuid.New(),
			Queued:       &queued,
			Started:      &started,
			Finished:     &finished,
			Error:        options.Error,
			Status:       options.Status,
		}
	}

	if createErr := db.Create(&tasks).Error; createErr != nil {
		return tasks, createErr
	}

	return tasks, nil
}
