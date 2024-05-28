package dao

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/candlepin_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/seeds"
	"github.com/openlyinc/pointy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type TemplateSuite struct {
	*DaoSuite
}

func TestTemplateSuite(t *testing.T) {
	m := DaoSuite{}
	testTemplateSuite := TemplateSuite{DaoSuite: &m}
	suite.Run(t, &testTemplateSuite)
}

func (s *TemplateSuite) TestCreate() {
	templateDao := templateDaoImpl{db: s.tx}

	orgID := orgIDTest
	err := seeds.SeedRepositoryConfigurations(s.tx, 2, seeds.SeedOptions{OrgID: orgID})
	assert.NoError(s.T(), err)

	var repoConfigs []models.RepositoryConfiguration
	err = s.tx.Where("org_id = ?", orgID).Find(&repoConfigs).Error
	assert.NoError(s.T(), err)

	timeNow := time.Now()
	reqTemplate := api.TemplateRequest{
		Name:            pointy.String("template test"),
		Description:     pointy.String("template test description"),
		RepositoryUUIDS: []string{repoConfigs[0].UUID, repoConfigs[1].UUID},
		Arch:            pointy.String(config.AARCH64),
		Version:         pointy.String(config.El8),
		Date:            &timeNow,
		OrgID:           &orgID,
	}

	respTemplate, err := templateDao.Create(context.Background(), reqTemplate)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), candlepin_client.GetEnvironmentID(respTemplate.UUID), respTemplate.ClientEnvironmentId)
	assert.Equal(s.T(), orgID, respTemplate.OrgID)
	assert.Equal(s.T(), *reqTemplate.Description, respTemplate.Description)
	assert.Equal(s.T(), timeNow.Round(time.Millisecond), respTemplate.Date.Round(time.Millisecond))
	assert.Equal(s.T(), *reqTemplate.Arch, respTemplate.Arch)
	assert.Equal(s.T(), *reqTemplate.Version, respTemplate.Version)
	assert.Len(s.T(), reqTemplate.RepositoryUUIDS, 2)
}

func (s *TemplateSuite) TestCreateDeleteCreateSameName() {
	templateDao := templateDaoImpl{db: s.tx}

	orgID := orgIDTest
	err := seeds.SeedRepositoryConfigurations(s.tx, 2, seeds.SeedOptions{OrgID: orgID})
	assert.NoError(s.T(), err)

	var repoConfigs []models.RepositoryConfiguration
	err = s.tx.Where("org_id = ?", orgID).Find(&repoConfigs).Error
	assert.NoError(s.T(), err)

	timeNow := time.Now()
	reqTemplate := api.TemplateRequest{
		Name:            pointy.String("template test"),
		Description:     pointy.String("template test description"),
		RepositoryUUIDS: []string{repoConfigs[0].UUID, repoConfigs[1].UUID},
		Arch:            pointy.String(config.AARCH64),
		Version:         pointy.String(config.El8),
		Date:            &timeNow,
		OrgID:           &orgID,
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
	assert.Equal(s.T(), timeNow.Round(time.Millisecond), respTemplate.Date.Round(time.Millisecond))
	assert.Equal(s.T(), *reqTemplate.Arch, respTemplate.Arch)
	assert.Equal(s.T(), *reqTemplate.Version, respTemplate.Version)
	assert.Len(s.T(), reqTemplate.RepositoryUUIDS, 2)
}

func (s *TemplateSuite) TestFetch() {
	templateDao := templateDaoImpl{db: s.tx}

	var found models.Template
	err := seeds.SeedTemplates(s.tx, 1, seeds.TemplateSeedOptions{OrgID: orgIDTest})
	assert.NoError(s.T(), err)

	err = s.tx.Where("org_id = ?", orgIDTest).First(&found).Error
	assert.NoError(s.T(), err)

	resp, err := templateDao.Fetch(context.Background(), orgIDTest, found.UUID)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), found.Name, resp.Name)
	assert.Equal(s.T(), found.OrgID, resp.OrgID)
	assert.Equal(s.T(), candlepin_client.GetEnvironmentID(resp.UUID), resp.ClientEnvironmentId)
}

func (s *TemplateSuite) TestFetchNotFound() {
	templateDao := templateDaoImpl{db: s.tx}

	var found models.Template
	err := seeds.SeedTemplates(s.tx, 1, seeds.TemplateSeedOptions{OrgID: orgIDTest})
	assert.NoError(s.T(), err)

	err = s.tx.Where("org_id = ?", orgIDTest).First(&found).Error
	assert.NoError(s.T(), err)

	_, err = templateDao.Fetch(context.Background(), orgIDTest, "bad uuid")
	daoError, ok := err.(*ce.DaoError)
	assert.True(s.T(), ok)
	assert.True(s.T(), daoError.NotFound)

	_, err = templateDao.Fetch(context.Background(), "bad orgID", found.UUID)
	daoError, ok = err.(*ce.DaoError)
	assert.True(s.T(), ok)
	assert.True(s.T(), daoError.NotFound)
}

func (s *TemplateSuite) TestList() {
	templateDao := templateDaoImpl{db: s.tx}
	var err error
	var found []models.Template
	var total int64

	s.seedWithRepoConfig(orgIDTest)

	err = s.tx.Where("org_id = ?", orgIDTest).Find(&found).Count(&total).Error
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), int64(2), total)

	responses, total, err := templateDao.List(context.Background(), orgIDTest, api.PaginationData{Limit: -1}, api.TemplateFilterData{})
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), int64(2), total)
	assert.Len(s.T(), responses.Data, 2)
	assert.Len(s.T(), responses.Data[0].RepositoryUUIDS, 2)
	assert.Equal(s.T(), candlepin_client.GetEnvironmentID(responses.Data[0].UUID), responses.Data[0].ClientEnvironmentId)
}

func (s *TemplateSuite) TestListNoTemplates() {
	templateDao := templateDaoImpl{db: s.tx}
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
	templateDao := templateDaoImpl{db: s.tx}
	var err error
	var found []models.Template
	var total int64

	err = seeds.SeedTemplates(s.tx, 20, seeds.TemplateSeedOptions{OrgID: orgIDTest})
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
	templateDao := templateDaoImpl{db: s.tx}
	var err error
	var found []models.Template
	var total int64

	arch := config.X8664
	version := config.El7
	options := seeds.TemplateSeedOptions{OrgID: orgIDTest, Arch: &arch, Version: &version}
	err = seeds.SeedTemplates(s.tx, 1, options)
	assert.NoError(s.T(), err)

	arch = config.S390x
	version = config.El8
	options = seeds.TemplateSeedOptions{OrgID: orgIDTest, Arch: &arch, Version: &version}
	err = seeds.SeedTemplates(s.tx, 1, options)
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
}

func (s *TemplateSuite) TestListFilterSearch() {
	templateDao := templateDaoImpl{db: s.tx}
	var err error
	var found []models.Template
	var total int64

	err = seeds.SeedTemplates(s.tx, 2, seeds.TemplateSeedOptions{OrgID: orgIDTest})
	assert.NoError(s.T(), err)

	err = s.tx.Where("org_id = ?", orgIDTest).Find(&found).Count(&total).Error
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), int64(2), total)

	filterData := api.TemplateFilterData{Search: found[0].Name[0:7]}
	responses, total, err := templateDao.List(context.Background(), orgIDTest, api.PaginationData{Limit: -1}, filterData)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), int64(1), total)
	assert.Len(s.T(), responses.Data, 1)
	assert.Equal(s.T(), found[0].Name, responses.Data[0].Name)
}

func (s *TemplateSuite) TestDelete() {
	templateDao := templateDaoImpl{db: s.tx}
	var err error
	var found models.Template

	err = seeds.SeedTemplates(s.tx, 1, seeds.TemplateSeedOptions{OrgID: orgIDTest})
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
	templateDao := templateDaoImpl{db: s.tx}
	var err error

	err = seeds.SeedTemplates(s.tx, 1, seeds.TemplateSeedOptions{OrgID: orgIDTest})
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
	templateDao := templateDaoImpl{db: s.tx}
	var err error
	var found models.Template

	err = seeds.SeedTemplates(s.tx, 1, seeds.TemplateSeedOptions{OrgID: orgIDTest})
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

func (s *TemplateSuite) fetchFirstTemplate() models.Template {
	var found models.Template
	err := s.tx.Where("org_id = ?", orgIDTest).Preload("RepositoryConfigurations").First(&found).Error
	assert.NoError(s.T(), err)
	return found
}

func (s *TemplateSuite) seedWithRepoConfig(orgId string) (models.Template, []string) {
	err := seeds.SeedRepositoryConfigurations(s.tx, 2, seeds.SeedOptions{OrgID: orgId})
	require.NoError(s.T(), err)

	var rcUUIDs []string
	err = s.tx.Model(models.RepositoryConfiguration{}).Where("org_id = ?", orgIDTest).Select("uuid").Find(&rcUUIDs).Error
	require.NoError(s.T(), err)

	err = seeds.SeedTemplates(s.tx, 2, seeds.TemplateSeedOptions{OrgID: orgId, RepositoryConfigUUIDs: rcUUIDs})
	require.NoError(s.T(), err)

	origTempl := s.fetchFirstTemplate()
	return origTempl, rcUUIDs
}

func (s *TemplateSuite) TestUpdate() {
	origTempl, rcUUIDs := s.seedWithRepoConfig(orgIDTest)

	templateDao := templateDaoImpl{db: s.tx}
	_, err := templateDao.Update(context.Background(), orgIDTest, origTempl.UUID, api.TemplateUpdateRequest{Description: pointy.Pointer("scratch"), RepositoryUUIDS: []string{rcUUIDs[0]}})
	require.NoError(s.T(), err)
	found := s.fetchFirstTemplate()
	// description does update
	assert.Equal(s.T(), "scratch", found.Description)
	assert.Equal(s.T(), 1, len(found.RepositoryConfigurations))
	assert.Equal(s.T(), rcUUIDs[0], found.RepositoryConfigurations[0].UUID)

	_, err = templateDao.Update(context.Background(), orgIDTest, found.UUID, api.TemplateUpdateRequest{RepositoryUUIDS: []string{rcUUIDs[1]}})
	require.NoError(s.T(), err)
	found = s.fetchFirstTemplate()
	assert.Equal(s.T(), 1, len(found.RepositoryConfigurations))
	assert.Equal(s.T(), rcUUIDs[1], found.RepositoryConfigurations[0].UUID)

	_, err = templateDao.Update(context.Background(), orgIDTest, found.UUID, api.TemplateUpdateRequest{RepositoryUUIDS: []string{"Notarealrepouuid"}})
	assert.Error(s.T(), err)
}

func (s *TemplateSuite) TestGetRepoChanges() {
	err := seeds.SeedRepositoryConfigurations(s.tx, 3, seeds.SeedOptions{OrgID: orgIDTest})
	assert.NoError(s.T(), err)

	var repoConfigs []models.RepositoryConfiguration
	s.tx.Model(&models.RepositoryConfiguration{}).Where("org_id = ?", orgIDTest).Find(&repoConfigs)

	templateDao := templateDaoImpl{db: s.tx}
	req := api.TemplateRequest{
		Name:            pointy.Pointer("test template"),
		RepositoryUUIDS: []string{repoConfigs[0].UUID, repoConfigs[1].UUID, repoConfigs[2].UUID},
		OrgID:           pointy.Pointer(orgIDTest),
	}
	resp, err := templateDao.Create(context.Background(), req)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), resp.Name, "test template")

	repoDistMap := map[string]string{}
	repoDistMap[repoConfigs[0].UUID] = "dist href"
	repoDistMap[repoConfigs[1].UUID] = "dist href"
	err = templateDao.UpdateDistributionHrefs(context.Background(), resp.UUID, resp.RepositoryUUIDS, repoDistMap)
	assert.NoError(s.T(), err)

	added, removed, unchanged, all, err := templateDao.GetRepoChanges(context.Background(), resp.UUID, []string{
		repoConfigs[0].UUID, repoConfigs[2].UUID})
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), []string{repoConfigs[2].UUID}, added)
	assert.Equal(s.T(), []string{repoConfigs[1].UUID}, removed)
	assert.Equal(s.T(), []string{repoConfigs[0].UUID}, unchanged)
	assert.ElementsMatch(s.T(), all, []string{repoConfigs[0].UUID, repoConfigs[1].UUID, repoConfigs[2].UUID})
}
