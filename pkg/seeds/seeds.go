package seeds

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"strconv"
	"time"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/google/uuid"
	"github.com/openlyinc/pointy"
	"gorm.io/gorm"
)

type SeedOptions struct {
	OrgID     string
	BatchSize int
	Arch      *string
	Versions  *[]string
	Status    *string
}

type IntrospectionStatusMetadata struct {
	status                       *string
	lastIntrospectionTime        *time.Time
	lastIntrospectionSuccessTime *time.Time
	lastIntrospectionUpdateTime  *time.Time
	lastIntrospectionError       *string
}

const (
	batchSize = 500
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

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

func SeedRepositoryConfigurations(db *gorm.DB, size int, options SeedOptions) error {
	var repos []models.Repository
	var repoConfigurations []models.RepositoryConfiguration

	if options.BatchSize != 0 {
		db.CreateBatchSize = options.BatchSize
	}

	for i := 0; i < size; i++ {
		introspectionMetadata := randomIntrospectionStatusMetadata(options.Status)
		repo := models.Repository{
			URL:                          randomURL(),
			LastIntrospectionTime:        introspectionMetadata.lastIntrospectionTime,
			LastIntrospectionSuccessTime: introspectionMetadata.lastIntrospectionSuccessTime,
			LastIntrospectionUpdateTime:  introspectionMetadata.lastIntrospectionUpdateTime,
			LastIntrospectionError:       introspectionMetadata.lastIntrospectionError,
			Status:                       *introspectionMetadata.status,
		}
		repos = append(repos, repo)
	}
	if err := db.Create(&repos).Error; err != nil {
		return err
	}

	for i := 0; i < size; i++ {
		repoConfig := models.RepositoryConfiguration{
			Name:           fmt.Sprintf("%s - %s - %s", RandStringBytes(2), "TestRepo", RandStringBytes(10)),
			Versions:       createVersionArray(options.Versions),
			Arch:           createArch(options.Arch),
			AccountID:      fmt.Sprintf("%d", rand.Intn(9999)),
			OrgID:          createOrgId(options.OrgID),
			RepositoryUUID: repos[i].UUID,
		}
		repoConfigurations = append(repoConfigurations, repoConfig)
	}
	if err := db.Create(&repoConfigurations).Error; err != nil {
		return errors.New("could not save seed")
	}
	return nil
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

func getIntrospectionTimestamps(status string) IntrospectionStatusMetadata {
	timestamp := time.Now()
	metadata := IntrospectionStatusMetadata{status: &status}

	switch status {
	case config.StatusValid:
		metadata.lastIntrospectionTime = &timestamp
		metadata.lastIntrospectionSuccessTime = &timestamp
		metadata.lastIntrospectionUpdateTime = &timestamp
	case config.StatusInvalid:
		metadata.lastIntrospectionError = pointy.String("bad introspection")
	case config.StatusUnavailable:
		metadata.lastIntrospectionTime = &timestamp
		metadata.lastIntrospectionSuccessTime = &timestamp
		metadata.lastIntrospectionUpdateTime = &timestamp
		metadata.lastIntrospectionError = pointy.String("bad introspection")
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
			Status:                       *introspectionMetadata.status,
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
		if r := db.Create(repos); r != nil && r.Error != nil {
			return r.Error
		}
		countRecords += len(repos)
		fmt.Printf("repoConfig: %d        \r", countRecords)
	}

	return nil
}

func SeedSnapshots(db *gorm.DB, repoUuid string, orgId string, size int) error {
	for i := 0; i < size; i++ {
		path := fmt.Sprintf("/seed/%v/%v", repoUuid, i)
		snap := models.Snapshot{
			VersionHref:      path,
			PublicationHref:  path,
			DistributionPath: path,
			DistributionHref: path,
			OrgId:            orgId,
			RepositoryUUID:   repoUuid,
			ContentCounts:    models.ContentCounts{},
		}
		res := db.Create(&snap)
		if res.Error != nil {
			return res.Error
		}
	}
	return nil
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
	AccountID string
	OrgID     string
	BatchSize int
	Status    string
	Error     *string
}

func SeedTasks(db *gorm.DB, size int, options TaskSeedOptions) error {
	if options.BatchSize != 0 {
		db.CreateBatchSize = options.BatchSize
	}

	orgId := createOrgId(options.OrgID)
	repoUUID := uuid.New()

	if options.AccountID != "" {
		repo := models.Repository{
			URL: randomURL(),
		}

		if err := db.Create(&repo).Error; err != nil {
			return err
		}

		repoUUID = uuid.MustParse(repo.UUID)

		repoConfig := models.RepositoryConfiguration{
			RepositoryUUID: repoUUID.String(),
			AccountID:      options.AccountID,
			Name:           fmt.Sprintf("%s - %s - %s", RandStringBytes(2), "TestRepo", RandStringBytes(10)),
			OrgID:          orgId,
		}

		if err := db.Create(&repoConfig).Error; err != nil {
			return err
		}
	}

	payloadData := map[string]string{"url": "https://example.com"}
	payload, err := json.Marshal(payloadData)
	if err != nil {
		return err
	}

	tasks := make([]models.TaskInfo, size)

	for i := 0; i < size; i++ {
		queued := time.Now().Add(time.Minute * time.Duration(i))
		started := time.Now().Add(time.Minute * time.Duration(i+5))
		finished := time.Now().Add(time.Minute * time.Duration(i+10))
		tasks[i] = models.TaskInfo{
			Id:             uuid.New(),
			Typename:       "example type",
			Payload:        payload,
			OrgId:          orgId,
			RepositoryUUID: repoUUID,
			Dependencies:   make([]uuid.UUID, 0),
			Token:          uuid.New(),
			Queued:         &queued,
			Started:        &started,
			Finished:       &finished,
			Error:          options.Error,
			Status:         options.Status,
		}
	}

	if createErr := db.Create(&tasks).Error; createErr != nil {
		return createErr
	}

	return nil
}
