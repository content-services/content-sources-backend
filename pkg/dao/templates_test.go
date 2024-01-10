package dao

import (
	"strings"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/seeds"
	"github.com/openlyinc/pointy"
	"github.com/stretchr/testify/assert"
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

	respTemplate, err := templateDao.Create(reqTemplate)
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
	err := seeds.SeedTemplates(s.tx, 1, seeds.SeedOptions{OrgID: orgIDTest})
	assert.NoError(s.T(), err)

	err = s.tx.Where("org_id = ?", orgIDTest).First(&found).Error
	assert.NoError(s.T(), err)

	resp, err := templateDao.Fetch(orgIDTest, found.UUID)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), found.Name, resp.Name)
	assert.Equal(s.T(), found.OrgID, resp.OrgID)
}

func (s *TemplateSuite) TestFetchNotFound() {
	templateDao := templateDaoImpl{db: s.tx}

	var found models.Template
	err := seeds.SeedTemplates(s.tx, 1, seeds.SeedOptions{OrgID: orgIDTest})
	assert.NoError(s.T(), err)

	err = s.tx.Where("org_id = ?", orgIDTest).First(&found).Error
	assert.NoError(s.T(), err)

	_, err = templateDao.Fetch(orgIDTest, "bad uuid")
	daoError, ok := err.(*ce.DaoError)
	assert.True(s.T(), ok)
	assert.True(s.T(), daoError.NotFound)

	_, err = templateDao.Fetch("bad orgID", found.UUID)
	daoError, ok = err.(*ce.DaoError)
	assert.True(s.T(), ok)
	assert.True(s.T(), daoError.NotFound)
}

func (s *TemplateSuite) TestList() {
	templateDao := templateDaoImpl{db: s.tx}
	var err error
	var found []models.Template
	var total int64

	err = seeds.SeedTemplates(s.tx, 2, seeds.SeedOptions{OrgID: orgIDTest})
	assert.NoError(s.T(), err)

	err = s.tx.Where("org_id = ?", orgIDTest).Find(&found).Count(&total).Error
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), int64(2), total)

	responses, total, err := templateDao.List(orgIDTest, api.PaginationData{Limit: -1}, api.TemplateFilterData{})
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), int64(2), total)
	assert.Len(s.T(), responses.Data, 2)
}

func (s *TemplateSuite) TestListNoTemplates() {
	templateDao := templateDaoImpl{db: s.tx}
	var err error
	var found []models.Template
	var total int64

	err = s.tx.Where("org_id = ?", orgIDTest).Find(&found).Count(&total).Error
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), int64(0), total)

	responses, total, err := templateDao.List(orgIDTest, api.PaginationData{Limit: -1}, api.TemplateFilterData{})
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), int64(0), total)
	assert.Len(s.T(), responses.Data, 0)
}

func (s *TemplateSuite) TestListPageLimit() {
	templateDao := templateDaoImpl{db: s.tx}
	var err error
	var found []models.Template
	var total int64

	err = seeds.SeedTemplates(s.tx, 20, seeds.SeedOptions{OrgID: orgIDTest})
	assert.NoError(s.T(), err)

	err = s.tx.Where("org_id = ?", orgIDTest).Find(&found).Count(&total).Error
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), int64(20), total)

	paginationData := api.PaginationData{Limit: 10}
	responses, total, err := templateDao.List(orgIDTest, paginationData, api.TemplateFilterData{})
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
	options := seeds.SeedOptions{OrgID: orgIDTest, Arch: &arch, Version: &version}
	err = seeds.SeedTemplates(s.tx, 1, options)
	assert.NoError(s.T(), err)

	arch = config.S390x
	version = config.El8
	options = seeds.SeedOptions{OrgID: orgIDTest, Arch: &arch, Version: &version}
	err = seeds.SeedTemplates(s.tx, 1, options)
	assert.NoError(s.T(), err)

	err = s.tx.Where("org_id = ?", orgIDTest).Find(&found).Count(&total).Error
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), int64(2), total)

	// Test filter by name
	filterData := api.TemplateFilterData{Name: found[0].Name}
	responses, total, err := templateDao.List(orgIDTest, api.PaginationData{Limit: -1}, filterData)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), int64(1), total)
	assert.Len(s.T(), responses.Data, 1)
	assert.Equal(s.T(), found[0].Name, responses.Data[0].Name)

	// Test filter by version
	filterData = api.TemplateFilterData{Version: found[0].Version}
	responses, total, err = templateDao.List(orgIDTest, api.PaginationData{Limit: -1}, filterData)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), int64(1), total)
	assert.Len(s.T(), responses.Data, 1)
	assert.Equal(s.T(), found[0].Version, responses.Data[0].Version)

	// Test filter by arch
	filterData = api.TemplateFilterData{Arch: found[0].Arch}
	responses, total, err = templateDao.List(orgIDTest, api.PaginationData{Limit: -1}, filterData)
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

	err = seeds.SeedTemplates(s.tx, 2, seeds.SeedOptions{OrgID: orgIDTest})
	assert.NoError(s.T(), err)

	err = s.tx.Where("org_id = ?", orgIDTest).Find(&found).Count(&total).Error
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), int64(2), total)

	filterData := api.TemplateFilterData{Search: found[0].Name[0:7]}
	responses, total, err := templateDao.List(orgIDTest, api.PaginationData{Limit: -1}, filterData)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), int64(1), total)
	assert.Len(s.T(), responses.Data, 1)
	assert.Equal(s.T(), found[0].Name, responses.Data[0].Name)
}
