package dao

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/candlepin_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/seeds"
	"github.com/content-services/content-sources-backend/pkg/utils"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type TemplateSuite struct {
	*DaoSuite
	pulpClient *pulp_client.MockPulpClient
}

func TestTemplateSuite(t *testing.T) {
	m := DaoSuite{}
	testTemplateSuite := TemplateSuite{DaoSuite: &m}
	suite.Run(t, &testTemplateSuite)
}

func (s *TemplateSuite) templateDao() templateDaoImpl {
	return templateDaoImpl{
		db:         s.tx,
		pulpClient: s.pulpClient,
	}
}
func (s *TemplateSuite) SetupTest() {
	s.DaoSuite.SetupTest()
	s.pulpClient = &pulp_client.MockPulpClient{}
	s.pulpClient.On("GetContentPath", context.Background()).Return(testContentPath, nil)
}
func (s *TemplateSuite) TestCreate() {
	templateDao := s.templateDao()

	orgID := orgIDTest
	_, err := seeds.SeedRepositoryConfigurations(s.tx, 2, seeds.SeedOptions{OrgID: orgID})
	assert.NoError(s.T(), err)

	var repoConfigs []models.RepositoryConfiguration
	err = s.tx.Where("org_id = ?", orgID).Find(&repoConfigs).Error
	assert.NoError(s.T(), err)

	s.createSnapshot(repoConfigs[0])
	s.createSnapshot(repoConfigs[1])

	timeNow := time.Now()
	reqTemplate := api.TemplateRequest{
		Name:            utils.Ptr("template test"),
		Description:     utils.Ptr("template test description"),
		RepositoryUUIDS: []string{repoConfigs[0].UUID, repoConfigs[1].UUID},
		Arch:            utils.Ptr(config.AARCH64),
		Version:         utils.Ptr(config.El8),
		Date:            (*api.EmptiableDate)(&timeNow),
		OrgID:           &orgID,
		UseLatest:       utils.Ptr(false),
	}

	respTemplate, err := templateDao.Create(context.Background(), reqTemplate)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), candlepin_client.GetEnvironmentID(respTemplate.UUID), respTemplate.RHSMEnvironmentID)
	assert.Equal(s.T(), orgID, respTemplate.OrgID)
	assert.Equal(s.T(), *reqTemplate.Description, respTemplate.Description)
	assert.True(s.T(), timeNow.Round(time.Millisecond).Equal(respTemplate.Date.Round(time.Millisecond)))
	assert.Equal(s.T(), *reqTemplate.Arch, respTemplate.Arch)
	assert.Equal(s.T(), *reqTemplate.Version, respTemplate.Version)
	assert.Len(s.T(), reqTemplate.RepositoryUUIDS, 2)
	assert.Equal(s.T(), *reqTemplate.UseLatest, respTemplate.UseLatest)
}

func (s *TemplateSuite) TestCreateDeleteCreateSameName() {
	templateDao := s.templateDao()

	orgID := orgIDTest
	_, err := seeds.SeedRepositoryConfigurations(s.tx, 2, seeds.SeedOptions{OrgID: orgID})
	assert.NoError(s.T(), err)

	var repoConfigs []models.RepositoryConfiguration
	err = s.tx.Where("org_id = ?", orgID).Find(&repoConfigs).Error
	assert.NoError(s.T(), err)

	s.createSnapshot(repoConfigs[0])
	s.createSnapshot(repoConfigs[1])

	timeNow := time.Now()
	reqTemplate := api.TemplateRequest{
		Name:            utils.Ptr("template test"),
		Description:     utils.Ptr("template test description"),
		RepositoryUUIDS: []string{repoConfigs[0].UUID, repoConfigs[1].UUID},
		Arch:            utils.Ptr(config.AARCH64),
		Version:         utils.Ptr(config.El8),
		Date:            (*api.EmptiableDate)(&timeNow),
		OrgID:           &orgID,
		User:            utils.Ptr("user"),
	}

	respTemplate, err := templateDao.Create(context.Background(), reqTemplate)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), orgID, respTemplate.OrgID)
	assert.Len(s.T(), reqTemplate.RepositoryUUIDS, 2)

	// As a template with this name exists, we expect this to error.
	_, expectedErr := templateDao.Create(context.Background(), reqTemplate)
	assert.Error(s.T(), expectedErr, "Template with this name already belongs to organization")

	// Delete the template
	err = templateDao.SoftDelete(context.Background(), respTemplate.OrgID, respTemplate.UUID)
	assert.NoError(s.T(), err)

	// We should now be able to recreate the template with the same name without issue
	respTemplate, err = templateDao.Create(context.Background(), reqTemplate)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), orgID, respTemplate.OrgID)
	assert.Equal(s.T(), *reqTemplate.Description, respTemplate.Description)
	assert.True(s.T(), timeNow.Round(time.Millisecond).Equal(respTemplate.Date.Round(time.Millisecond)))
	assert.Equal(s.T(), *reqTemplate.Arch, respTemplate.Arch)
	assert.Equal(s.T(), *reqTemplate.Version, respTemplate.Version)
	assert.Len(s.T(), reqTemplate.RepositoryUUIDS, 2)
	assert.Equal(s.T(), *reqTemplate.User, respTemplate.CreatedBy)
	assert.Equal(s.T(), *reqTemplate.User, respTemplate.LastUpdatedBy)
}

func (s *TemplateSuite) TestFetch() {
	templateDao := s.templateDao()

	var found models.Template
	_, err := seeds.SeedTemplates(s.tx, 1, seeds.TemplateSeedOptions{OrgID: orgIDTest})
	assert.NoError(s.T(), err)

	err = s.tx.Where("org_id = ?", orgIDTest).First(&found).Error
	assert.NoError(s.T(), err)

	resp, err := templateDao.Fetch(context.Background(), orgIDTest, found.UUID, false)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), found.Name, resp.Name)
	assert.Equal(s.T(), found.OrgID, resp.OrgID)
	assert.Equal(s.T(), candlepin_client.GetEnvironmentID(resp.UUID), resp.RHSMEnvironmentID)
	assert.Equal(s.T(), found.LastUpdatedBy, resp.LastUpdatedBy)
	assert.Equal(s.T(), found.CreatedBy, resp.CreatedBy)
	assert.True(s.T(), found.CreatedAt.Equal(resp.CreatedAt))
	assert.True(s.T(), found.UpdatedAt.Equal(resp.UpdatedAt))
}

func (s *TemplateSuite) TestFetchSoftDeleted() {
	templateDao := s.templateDao()

	var found models.Template
	_, err := seeds.SeedTemplates(s.tx, 1, seeds.TemplateSeedOptions{OrgID: orgIDTest})
	assert.NoError(s.T(), err)

	err = s.tx.Where("org_id = ?", orgIDTest).First(&found).Error
	assert.NoError(s.T(), err)

	resp, err := templateDao.Fetch(context.Background(), orgIDTest, found.UUID, false)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), found.Name, resp.Name)

	err = s.tx.Delete(&found).Error
	assert.NoError(s.T(), err)

	resp, err = templateDao.Fetch(context.Background(), orgIDTest, found.UUID, false)
	assert.ErrorContains(s.T(), err, "not found")

	resp, err = templateDao.Fetch(context.Background(), orgIDTest, found.UUID, true)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), found.Name, resp.Name)
}

func (s *TemplateSuite) TestFetchSoftDeletedRepoConfig() {
	templateDao := s.templateDao()
	found := models.Template{}

	template, rcIds := s.seedWithRepoConfig(orgIDTest, 1, false)

	err := s.tx.Where("org_id = ?", orgIDTest).First(&found).Error
	assert.NoError(s.T(), err)

	resp, err := templateDao.Fetch(context.Background(), orgIDTest, found.UUID, false)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), found.Name, resp.Name)

	assert.Equal(s.T(), 2, len(resp.RepositoryUUIDS))

	update := api.TemplateUpdateRequest{RepositoryUUIDS: []string{rcIds[0]}}
	_, err = templateDao.Update(context.Background(), orgIDTest, template.UUID, update)
	assert.NoError(s.T(), err)

	resp, err = templateDao.Fetch(context.Background(), orgIDTest, found.UUID, false)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), 1, len(resp.RepositoryUUIDS))

	resp, err = templateDao.Fetch(context.Background(), orgIDTest, found.UUID, false)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), found.Name, resp.Name)
	assert.Equal(s.T(), 1, len(resp.RepositoryUUIDS))
}

func (s *TemplateSuite) TestFetchNotFound() {
	templateDao := s.templateDao()

	var found models.Template
	_, err := seeds.SeedTemplates(s.tx, 1, seeds.TemplateSeedOptions{OrgID: orgIDTest})
	assert.NoError(s.T(), err)

	err = s.tx.Where("org_id = ?", orgIDTest).First(&found).Error
	assert.NoError(s.T(), err)

	_, err = templateDao.Fetch(context.Background(), orgIDTest, "bad uuid", false)
	daoError, ok := err.(*ce.DaoError)
	assert.True(s.T(), ok)
	assert.True(s.T(), daoError.NotFound)

	_, err = templateDao.Fetch(context.Background(), "bad orgID", found.UUID, false)
	daoError, ok = err.(*ce.DaoError)
	assert.True(s.T(), ok)
	assert.True(s.T(), daoError.NotFound)
}

func (s *TemplateSuite) TestList() {
	templateDao := s.templateDao()
	var err error
	var found []models.Template
	var total int64

	s.seedWithRepoConfig(orgIDTest, 1, false)

	err = s.tx.Where("org_id = ?", orgIDTest).Find(&found).Count(&total).Error
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), int64(1), total)

	responses, total, err := templateDao.List(context.Background(), orgIDTest, api.PaginationData{Limit: -1}, api.TemplateFilterData{})
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), int64(1), total)
	assert.Len(s.T(), responses.Data, 1)
	assert.Len(s.T(), responses.Data[0].RepositoryUUIDS, 2)
	assert.Equal(s.T(), candlepin_client.GetEnvironmentID(responses.Data[0].UUID), responses.Data[0].RHSMEnvironmentID)
	assert.Equal(s.T(), responses.Data[0].CreatedBy, found[0].CreatedBy)
	assert.Equal(s.T(), responses.Data[0].LastUpdatedBy, found[0].LastUpdatedBy)
	assert.True(s.T(), responses.Data[0].CreatedAt.Equal(found[0].CreatedAt))
	assert.True(s.T(), responses.Data[0].UpdatedAt.Equal(found[0].UpdatedAt))
	assert.Equal(s.T(), responses.Data[0].UseLatest, found[0].UseLatest)
	assert.Equal(s.T(), 2, len(responses.Data[0].Snapshots))
	assert.NotEmpty(s.T(), responses.Data[0].Snapshots[0].UUID)
	assert.NotEmpty(s.T(), responses.Data[0].Snapshots[0].RepositoryName)
	assert.NotEmpty(s.T(), responses.Data[0].Snapshots[0].URL)
}

func (s *TemplateSuite) TestListNoTemplates() {
	templateDao := s.templateDao()
	var err error
	var found []models.Template
	var total int64

	err = s.tx.Where("org_id = ?", orgIDTest).Find(&found).Count(&total).Error
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), int64(0), total)

	responses, total, err := templateDao.List(context.Background(), orgIDTest, api.PaginationData{Limit: -1}, api.TemplateFilterData{})
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), int64(0), total)
	assert.Len(s.T(), responses.Data, 0)
}

func (s *TemplateSuite) TestListPageLimit() {
	templateDao := s.templateDao()
	var err error
	var found []models.Template
	var total int64

	_, err = seeds.SeedTemplates(s.tx, 20, seeds.TemplateSeedOptions{OrgID: orgIDTest})
	assert.NoError(s.T(), err)

	err = s.tx.Where("org_id = ?", orgIDTest).Find(&found).Count(&total).Error
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), int64(20), total)

	paginationData := api.PaginationData{Limit: 10}
	responses, total, err := templateDao.List(context.Background(), orgIDTest, paginationData, api.TemplateFilterData{})
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), int64(20), total)
	assert.Len(s.T(), responses.Data, 10)

	// Asserts that the first item is lower (alphabetically a-z) than the last item.
	firstItem := strings.ToLower(responses.Data[0].Name)
	lastItem := strings.ToLower(responses.Data[len(responses.Data)-1].Name)
	assert.Equal(s.T(), -1, strings.Compare(firstItem, lastItem))
}

func (s *TemplateSuite) TestListFilters() {
	templateDao := s.templateDao()
	var err error
	var found []models.Template
	var total int64

	arch := config.X8664
	version := config.El7
	options := seeds.TemplateSeedOptions{OrgID: orgIDTest, Arch: &arch, Version: &version}
	_, err = seeds.SeedTemplates(s.tx, 1, options)
	assert.NoError(s.T(), err)

	arch = config.S390x
	version = config.El8
	options = seeds.TemplateSeedOptions{OrgID: orgIDTest, Arch: &arch, Version: &version}
	_, err = seeds.SeedTemplates(s.tx, 1, options)
	assert.NoError(s.T(), err)

	err = s.tx.Where("org_id = ?", orgIDTest).Find(&found).Count(&total).Error
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), int64(2), total)

	// Test filter by name
	filterData := api.TemplateFilterData{Name: found[0].Name}
	responses, total, err := templateDao.List(context.Background(), orgIDTest, api.PaginationData{Limit: -1}, filterData)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), int64(1), total)
	assert.Len(s.T(), responses.Data, 1)
	assert.Equal(s.T(), found[0].Name, responses.Data[0].Name)

	// Test filter by version
	filterData = api.TemplateFilterData{Version: found[0].Version}
	responses, total, err = templateDao.List(context.Background(), orgIDTest, api.PaginationData{Limit: -1}, filterData)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), int64(1), total)
	assert.Len(s.T(), responses.Data, 1)
	assert.Equal(s.T(), found[0].Version, responses.Data[0].Version)

	// Test filter by arch
	filterData = api.TemplateFilterData{Arch: found[0].Arch}
	responses, total, err = templateDao.List(context.Background(), orgIDTest, api.PaginationData{Limit: -1}, filterData)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), int64(1), total)
	assert.Len(s.T(), responses.Data, 1)
	assert.Equal(s.T(), found[0].Arch, responses.Data[0].Arch)

	// Test Filter by RepositoryUUIDs
	template, rcUUIDs := s.seedWithRepoConfig(orgIDTest, 2, false)
	filterData = api.TemplateFilterData{RepositoryUUIDs: []string{rcUUIDs[0], rcUUIDs[1]}}
	responses, total, err = templateDao.List(context.Background(), orgIDTest, api.PaginationData{Limit: -1}, filterData)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), int64(2), total)
	assert.Len(s.T(), responses.Data, 2)
	assert.True(s.T(), template.UUID == responses.Data[0].UUID || template.UUID == responses.Data[1].UUID)
	assert.True(s.T(), rcUUIDs[0] == responses.Data[0].RepositoryUUIDS[0] || rcUUIDs[0] == responses.Data[1].RepositoryUUIDS[0])
	assert.True(s.T(), rcUUIDs[1] == responses.Data[0].RepositoryUUIDS[1] || rcUUIDs[1] == responses.Data[1].RepositoryUUIDS[1])
}

func (s *TemplateSuite) TestListFilterSearch() {
	templateDao := s.templateDao()
	var err error
	var found []models.Template
	var total int64

	_, err = seeds.SeedTemplates(s.tx, 2, seeds.TemplateSeedOptions{OrgID: orgIDTest})
	assert.NoError(s.T(), err)

	err = s.tx.Where("org_id = ?", orgIDTest).Find(&found).Count(&total).Error
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), int64(2), total)

	filterData := api.TemplateFilterData{Search: found[0].Name[0:7]}
	responses, total, err := templateDao.List(context.Background(), orgIDTest, api.PaginationData{Limit: -1}, filterData)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), int64(1), total)
	assert.Len(s.T(), responses.Data, 1)
}

func (s *TemplateSuite) TestListBySnapshot() {
	templateDao := s.templateDao()
	mockPulpClient := pulp_client.NewMockPulpClient(s.T())
	var err error
	var found []models.Template
	var total int64

	mockPulpClient.On("GetContentPath", context.Background()).Return(testContentPath, nil)

	repos, err := seeds.SeedRepositoryConfigurations(s.tx, 2, seeds.SeedOptions{OrgID: orgIDTest})
	assert.NoError(s.T(), err)
	r1 := repos[0]
	r1snaps, err := seeds.SeedSnapshots(s.tx, r1.UUID, 2)
	assert.NoError(s.T(), err)
	r2 := repos[1]
	r2snaps, err := seeds.SeedSnapshots(s.tx, r2.UUID, 1)
	assert.NoError(s.T(), err)

	var t1snaps []models.Snapshot
	_, err = seeds.SeedTemplates(s.tx, 1, seeds.TemplateSeedOptions{
		OrgID:                 orgIDTest,
		RepositoryConfigUUIDs: []string{r1.UUID, r2.UUID},
		Snapshots:             append(t1snaps, r1snaps[1], r2snaps[0]),
	})
	assert.NoError(s.T(), err)
	_, err = seeds.SeedTemplates(s.tx, 1, seeds.TemplateSeedOptions{
		OrgID:                 orgIDTest,
		RepositoryConfigUUIDs: []string{r2.UUID},
		Snapshots:             r2snaps,
	})
	assert.NoError(s.T(), err)

	err = s.tx.Where("org_id = ?", orgIDTest).Find(&found).Count(&total).Error
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), int64(2), total)

	filterData := api.TemplateFilterData{SnapshotUUIDs: []string{r2snaps[0].UUID}}
	responses, total, err := templateDao.List(context.Background(), orgIDTest, api.PaginationData{Limit: -1}, filterData)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), int64(2), total)
	assert.Len(s.T(), responses.Data, 2)

	filterData = api.TemplateFilterData{SnapshotUUIDs: []string{r1snaps[1].UUID}}
	responses, total, err = templateDao.List(context.Background(), orgIDTest, api.PaginationData{Limit: -1}, filterData)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), int64(1), total)
	assert.Len(s.T(), responses.Data, 1)

	filterData = api.TemplateFilterData{
		RepositoryUUIDs: []string{r2.UUID},
		SnapshotUUIDs:   []string{r1snaps[1].UUID},
	}
	responses, total, err = templateDao.List(context.Background(), orgIDTest, api.PaginationData{Limit: -1}, filterData)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), int64(2), total)
	assert.Len(s.T(), responses.Data, 2)
}

func (s *TemplateSuite) TestListToBeDeletedSnapshots() {
	templateDao := s.templateDao()
	var err error
	var found []models.Template
	var total int64

	s.seedWithRepoConfig(orgIDTest, 1, true)

	err = s.tx.Where("org_id = ?", orgIDTest).Find(&found).Count(&total).Error
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), int64(1), total)

	responses, total, err := templateDao.List(context.Background(), orgIDTest, api.PaginationData{Limit: -1}, api.TemplateFilterData{})
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), int64(1), total)
	assert.Len(s.T(), responses.Data, 1)
	assert.Len(s.T(), responses.Data[0].RepositoryUUIDS, 2)
	assert.Equal(s.T(), 2, len(responses.Data[0].Snapshots))
	assert.Equal(s.T(), 1, len(responses.Data[0].ToBeDeletedSnapshots))
	assert.True(s.T(), responses.Data[0].ToBeDeletedSnapshots[0].CreatedAt.Before(time.Now().Add(-time.Duration(config.Get().Options.SnapshotRetainDaysLimit-14)*24*time.Hour)))
	assert.True(s.T(), responses.Data[0].ToBeDeletedSnapshots[0].UUID == responses.Data[0].Snapshots[0].UUID || responses.Data[0].ToBeDeletedSnapshots[0].UUID == responses.Data[0].Snapshots[1].UUID)
}

func (s *TemplateSuite) TestDelete() {
	templateDao := s.templateDao()
	var err error
	var found models.Template

	_, err = seeds.SeedTemplates(s.tx, 1, seeds.TemplateSeedOptions{OrgID: orgIDTest})
	assert.Nil(s.T(), err)

	template := models.Template{}
	err = s.tx.
		First(&template, "org_id = ?", orgIDTest).
		Error
	require.NoError(s.T(), err)

	err = templateDao.SoftDelete(context.Background(), template.OrgID, template.UUID)
	assert.NoError(s.T(), err)

	err = s.tx.
		First(&found, "org_id = ? AND uuid = ?", template.OrgID, template.UUID).
		Error
	require.Error(s.T(), err)
	assert.Equal(s.T(), "record not found", err.Error())

	err = s.tx.Unscoped().
		First(&found, "org_id = ? AND uuid = ?", template.OrgID, template.UUID).
		Error
	require.NoError(s.T(), err)

	err = templateDao.Delete(context.Background(), template.OrgID, template.UUID)
	assert.NoError(s.T(), err)

	err = s.tx.
		First(&found, "org_id = ? AND uuid = ?", template.OrgID, template.UUID).
		Error
	require.Error(s.T(), err)
	assert.Equal(s.T(), "record not found", err.Error())
}

func (s *TemplateSuite) TestDeleteNotFound() {
	templateDao := s.templateDao()
	var err error

	_, err = seeds.SeedTemplates(s.tx, 1, seeds.TemplateSeedOptions{OrgID: orgIDTest})
	assert.Nil(s.T(), err)

	found := models.Template{}
	err = s.tx.
		First(&found, "org_id = ?", orgIDTest).
		Error
	require.NoError(s.T(), err)

	err = templateDao.SoftDelete(context.Background(), "bad org id", found.UUID)
	assert.Error(s.T(), err)
	daoError, ok := err.(*ce.DaoError)
	assert.True(s.T(), ok)
	assert.True(s.T(), daoError.NotFound)

	err = templateDao.SoftDelete(context.Background(), orgIDTest, "bad uuid")
	assert.Error(s.T(), err)
	daoError, ok = err.(*ce.DaoError)
	assert.True(s.T(), ok)
	assert.True(s.T(), daoError.NotFound)

	err = templateDao.Delete(context.Background(), "bad org id", found.UUID)
	assert.Error(s.T(), err)
	daoError, ok = err.(*ce.DaoError)
	assert.True(s.T(), ok)
	assert.True(s.T(), daoError.NotFound)

	err = templateDao.Delete(context.Background(), orgIDTest, "bad uuid")
	assert.Error(s.T(), err)
	daoError, ok = err.(*ce.DaoError)
	assert.True(s.T(), ok)
	assert.True(s.T(), daoError.NotFound)
}

func (s *TemplateSuite) TestClearDeletedAt() {
	templateDao := s.templateDao()
	var err error
	var found models.Template

	_, err = seeds.SeedTemplates(s.tx, 1, seeds.TemplateSeedOptions{OrgID: orgIDTest})
	assert.Nil(s.T(), err)

	template := models.Template{}
	err = s.tx.
		First(&template, "org_id = ?", orgIDTest).
		Error
	require.NoError(s.T(), err)

	err = templateDao.ClearDeletedAt(context.Background(), template.OrgID, template.UUID)
	assert.NoError(s.T(), err)

	err = s.tx.
		First(&found, "org_id = ? AND uuid = ?", template.OrgID, template.UUID).
		Where("deleted_at = ?", nil).
		Error
	require.NoError(s.T(), err)
}

func (s *TemplateSuite) fetchTemplate(uuid string) models.Template {
	var found models.Template
	err := s.tx.Where("uuid = ? AND org_id = ?", uuid, orgIDTest).Preload("TemplateRepositoryConfigurations").First(&found).Error
	assert.NoError(s.T(), err)
	return found
}

func (s *TemplateSuite) seedWithRepoConfig(orgId string, templateSize int, withToBeDeleted bool) (models.Template, []string) {
	repoConfigs, err := seeds.SeedRepositoryConfigurations(s.tx, 2, seeds.SeedOptions{OrgID: orgId})
	require.NoError(s.T(), err)

	var rcUUIDs []string
	err = s.tx.Model(models.RepositoryConfiguration{}).Where("org_id = ?", orgIDTest).Select("uuid").Find(&rcUUIDs).Error
	require.NoError(s.T(), err)

	var snap1 models.Snapshot
	if withToBeDeleted {
		snap1 = s.createSnapshotAtSpecifiedTime(repoConfigs[0], time.Now().Add(-time.Duration(config.Get().Options.SnapshotRetainDaysLimit-5)*24*time.Hour))
		_ = s.createSnapshot(repoConfigs[0])
	} else {
		snap1 = s.createSnapshot(repoConfigs[0])
	}
	snap3 := s.createSnapshot(repoConfigs[1])

	templates, err := seeds.SeedTemplates(s.tx, templateSize, seeds.TemplateSeedOptions{OrgID: orgId, RepositoryConfigUUIDs: rcUUIDs, Snapshots: []models.Snapshot{snap1, snap3}})
	require.NoError(s.T(), err)

	return templates[0], rcUUIDs
}

func (s *TemplateSuite) createSnapshot(rConfig models.RepositoryConfiguration) models.Snapshot {
	t := s.T()
	tx := s.tx

	snap := models.Snapshot{
		Base:                        models.Base{},
		VersionHref:                 "/pulp/version",
		PublicationHref:             "/pulp/publication",
		DistributionPath:            fmt.Sprintf("/path/to/%v", uuid.NewString()),
		RepositoryConfigurationUUID: rConfig.UUID,
		ContentCounts:               models.ContentCountsType{"rpm.package": int64(3), "rpm.advisory": int64(1)},
		AddedCounts:                 models.ContentCountsType{"rpm.package": int64(1), "rpm.advisory": int64(3)},
		RemovedCounts:               models.ContentCountsType{"rpm.package": int64(2), "rpm.advisory": int64(2)},
	}

	sDao := snapshotDaoImpl{db: tx}
	err := sDao.Create(context.Background(), &snap)
	assert.NoError(t, err)
	return snap
}

func (s *TemplateSuite) createSnapshotAtSpecifiedTime(rConfig models.RepositoryConfiguration, CreatedAt time.Time) models.Snapshot {
	t := s.T()
	tx := s.tx

	snap := models.Snapshot{
		Base:                        models.Base{CreatedAt: CreatedAt},
		VersionHref:                 "/pulp/version",
		PublicationHref:             "/pulp/publication",
		DistributionPath:            fmt.Sprintf("/path/to/%v", uuid.NewString()),
		RepositoryConfigurationUUID: rConfig.UUID,
		ContentCounts:               models.ContentCountsType{"rpm.package": int64(3), "rpm.advisory": int64(1)},
		AddedCounts:                 models.ContentCountsType{"rpm.package": int64(1), "rpm.advisory": int64(3)},
		RemovedCounts:               models.ContentCountsType{"rpm.package": int64(2), "rpm.advisory": int64(2)},
	}

	sDao := snapshotDaoImpl{db: tx}
	err := sDao.Create(context.Background(), &snap)
	assert.NoError(t, err)
	return snap
}

func (s *TemplateSuite) TestUpdate() {
	origTempl, rcUUIDs := s.seedWithRepoConfig(orgIDTest, 2, false)

	var repoConfigs []models.RepositoryConfiguration
	err := s.tx.Where("org_id = ?", orgIDTest).Find(&repoConfigs).Error
	assert.NoError(s.T(), err)

	templateDao := s.templateDao()
	_, err = templateDao.Update(context.Background(), orgIDTest, origTempl.UUID, api.TemplateUpdateRequest{Description: utils.Ptr("scratch"), RepositoryUUIDS: []string{rcUUIDs[0]}, Name: utils.Ptr("test-name")})
	require.NoError(s.T(), err)
	found := s.fetchTemplate(origTempl.UUID)
	// description, name
	assert.Equal(s.T(), "scratch", found.Description)
	assert.Equal(s.T(), "test-name", found.Name)
	assert.Equal(s.T(), 1, len(found.TemplateRepositoryConfigurations))
	assert.Equal(s.T(), rcUUIDs[0], found.TemplateRepositoryConfigurations[0].RepositoryConfigurationUUID)

	_, err = templateDao.Update(context.Background(), orgIDTest, found.UUID, api.TemplateUpdateRequest{RepositoryUUIDS: []string{rcUUIDs[1]}})
	require.NoError(s.T(), err)
	found = s.fetchTemplate(origTempl.UUID)
	assert.Equal(s.T(), 1, len(found.TemplateRepositoryConfigurations))
	assert.Equal(s.T(), rcUUIDs[1], found.TemplateRepositoryConfigurations[0].RepositoryConfigurationUUID)

	// Test repo is validated
	_, err = templateDao.Update(context.Background(), orgIDTest, found.UUID, api.TemplateUpdateRequest{RepositoryUUIDS: []string{"Notarealrepouuid"}})
	assert.Error(s.T(), err)

	// Test user is updated
	_, err = templateDao.Update(context.Background(), orgIDTest, found.UUID, api.TemplateUpdateRequest{RepositoryUUIDS: []string{rcUUIDs[1]}, User: utils.Ptr("new user")})
	require.NoError(s.T(), err)
	found = s.fetchTemplate(origTempl.UUID)
	assert.Equal(s.T(), "new user", found.LastUpdatedBy)

	// Test use_latest validation error
	now := time.Now()
	_, err = templateDao.Update(context.Background(), orgIDTest, found.UUID, api.TemplateUpdateRequest{Date: utils.Ptr(api.EmptiableDate(now)), UseLatest: utils.Ptr(true)})
	assert.Error(s.T(), err)

	// Test use_latest is updated
	_, err = templateDao.Update(context.Background(), orgIDTest, found.UUID, api.TemplateUpdateRequest{Date: utils.Ptr(api.EmptiableDate(time.Time{})), UseLatest: utils.Ptr(true)})
	require.NoError(s.T(), err)
	found = s.fetchTemplate(origTempl.UUID)
	assert.Equal(s.T(), true, found.UseLatest)
}

func (s *TemplateSuite) TestGetRepoChanges() {
	_, err := seeds.SeedRepositoryConfigurations(s.tx, 3, seeds.SeedOptions{OrgID: orgIDTest})
	assert.NoError(s.T(), err)

	var repoConfigs []models.RepositoryConfiguration
	s.tx.Model(&models.RepositoryConfiguration{}).Where("org_id = ?", orgIDTest).Find(&repoConfigs)

	snap1 := s.createSnapshot(repoConfigs[0])
	snap2 := s.createSnapshot(repoConfigs[1])
	snap3 := s.createSnapshot(repoConfigs[2])

	templateDao := s.templateDao()
	req := api.TemplateRequest{
		Name:            utils.Ptr("test template"),
		RepositoryUUIDS: []string{repoConfigs[0].UUID, repoConfigs[1].UUID, repoConfigs[2].UUID},
		OrgID:           utils.Ptr(orgIDTest),
		Arch:            utils.Ptr(config.AARCH64),
		Version:         utils.Ptr(config.El8),
	}
	resp, err := templateDao.Create(context.Background(), req)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), resp.Name, "test template")

	repoDistMap := map[string]string{}
	repoDistMap[repoConfigs[0].UUID] = "dist href"
	repoDistMap[repoConfigs[1].UUID] = "dist href"
	err = templateDao.UpdateDistributionHrefs(context.Background(), resp.UUID, resp.RepositoryUUIDS, []models.Snapshot{snap1, snap2, snap3}, repoDistMap)
	assert.NoError(s.T(), err)

	added, removed, unchanged, all, err := templateDao.GetRepoChanges(context.Background(), resp.UUID, []string{
		repoConfigs[0].UUID, repoConfigs[2].UUID})
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), []string{repoConfigs[2].UUID}, added)
	assert.Equal(s.T(), []string{repoConfigs[1].UUID}, removed)
	assert.Equal(s.T(), []string{repoConfigs[0].UUID}, unchanged)
	assert.ElementsMatch(s.T(), all, []string{repoConfigs[0].UUID, repoConfigs[1].UUID, repoConfigs[2].UUID})
}

func (s *TemplateSuite) TestUpdateLastUpdateTask() {
	template, _ := s.seedWithRepoConfig(orgIDTest, 1, false)

	templateDao := s.templateDao()
	taskUUID := uuid.NewString()
	err := templateDao.UpdateLastUpdateTask(context.Background(), taskUUID, orgIDTest, template.UUID)
	require.NoError(s.T(), err)

	found := s.fetchTemplate(template.UUID)
	assert.Equal(s.T(), taskUUID, found.LastUpdateTaskUUID)
}

func (s *TemplateSuite) TestUpdateLastError() {
	template, _ := s.seedWithRepoConfig(orgIDTest, 1, false)

	templateDao := s.templateDao()
	lastUpdateSnapshotError := "test error"
	err := templateDao.UpdateLastError(context.Background(), orgIDTest, template.UUID, lastUpdateSnapshotError)
	require.NoError(s.T(), err)

	found := s.fetchTemplate(template.UUID)
	assert.Equal(s.T(), lastUpdateSnapshotError, *found.LastUpdateSnapshotError)
}

func (s *TemplateSuite) TestSetEnvironmentCreated() {
	template, _ := s.seedWithRepoConfig(orgIDTest, 1, false)
	assert.False(s.T(), template.RHSMEnvironmentCreated)
	templateDao := s.templateDao()
	err := templateDao.SetEnvironmentCreated(context.Background(), template.UUID)
	require.NoError(s.T(), err)
	found := s.fetchTemplate(template.UUID)
	assert.True(s.T(), found.RHSMEnvironmentCreated)
}

func (s *TemplateSuite) TestGetRepositoryConfigurationFile() {
	t := s.T()
	tx := s.tx
	ctx := context.Background()

	testRepository := models.Repository{
		URL:                    "https://example.com",
		LastIntrospectionTime:  nil,
		LastIntrospectionError: nil,
	}
	err := tx.Create(&testRepository).Error
	assert.NoError(t, err)

	repoConfig := models.RepositoryConfiguration{
		Name:           "test",
		OrgID:          orgIDTest,
		RepositoryUUID: testRepository.UUID,
	}
	err = tx.Create(&repoConfig).Error
	assert.NoError(t, err)
	expectedRepoID := "[test]"

	s.createSnapshot(repoConfig)
	templateDao := s.templateDao()
	req := api.TemplateRequest{
		Name:            utils.Ptr("test template"),
		RepositoryUUIDS: []string{repoConfig.UUID},
		OrgID:           utils.Ptr(orgIDTest),
		Arch:            utils.Ptr(config.AARCH64),
		Version:         utils.Ptr(config.El8),
	}
	template, err := templateDao.Create(ctx, req)
	assert.NoError(t, err)

	repoConfigFile, err := templateDao.GetRepositoryConfigurationFile(ctx, template.OrgID, template.UUID)
	assert.NoError(t, err)
	assert.Contains(t, repoConfigFile, repoConfig.Name)
	assert.Contains(t, repoConfigFile, expectedRepoID)
	assert.Contains(t, repoConfigFile, testContentPath)
	assert.Contains(t, repoConfigFile, template.UUID)
	assert.Contains(t, repoConfigFile, "module_hotfixes=0")
}
