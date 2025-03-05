package dao

import (
	"context"
	"sync"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/clients/candlepin_client"
	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/models"
	uuid2 "github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type DomainSuite struct {
	*DaoSuite
}

func TestDomainSuite(t *testing.T) {
	m := DaoSuite{}
	r := DomainSuite{DaoSuite: &m}
	suite.Run(t, &r)
}

func (ds *DomainSuite) TestCreate() {
	orgId := "DomainSuiteTest"
	dd := domainDaoImpl{db: ds.tx}

	name, err := dd.Create(context.Background(), orgId)
	assert.NoError(ds.T(), err)
	assert.NotEmpty(ds.T(), name)
	// try again
	name, err = dd.Create(context.Background(), orgId)
	assert.NoError(ds.T(), err)
	assert.NotEmpty(ds.T(), name)

	name, err = dd.Fetch(context.Background(), orgId)
	assert.NoError(ds.T(), err)
	assert.NotEmpty(ds.T(), name)
}

func (ds *DomainSuite) TestList() {
	dd := domainDaoImpl{db: ds.tx}
	numOrgs := 5

	var existingOrgs []models.Domain
	res := ds.tx.Model(&models.Domain{}).Find(&existingOrgs)
	assert.NoError(ds.T(), res.Error)

	newOrgs := make([]models.Domain, numOrgs)
	for i := 0; i < numOrgs; i++ {
		orgID := randomHexadecimal(10)

		name, err := dd.Create(context.Background(), orgID)
		assert.NoError(ds.T(), err)
		assert.NotEmpty(ds.T(), name)

		newOrgs[i].OrgId = orgID
		newOrgs[i].DomainName = name
	}

	expectedOrgs := append(newOrgs, existingOrgs...)
	expectedCount := len(newOrgs) + len(existingOrgs)

	orgs, err := dd.List(context.Background())
	assert.NoError(ds.T(), err)
	assert.Equal(ds.T(), expectedCount, len(orgs))
	assert.ElementsMatch(ds.T(), expectedOrgs, orgs)
}

func TestConcurrentGetDomainName(t *testing.T) {
	// Note, this test does not use a transaction, as it fails when multiple go routines are trying to do that
	orgId := uuid2.NewString()
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			dDao := GetDomainDao(db.DB, pulp_client.NewMockPulpGlobalClient(t), candlepin_client.NewCandlepinClient())
			dName, err := dDao.FetchOrCreateDomain(context.Background(), orgId)
			assert.NoError(t, err)
			assert.NotEmpty(t, dName)
			wg.Done()
		}()
	}
	wg.Wait()
}

func (ds *DomainSuite) TestDelete() {
	orgId := "DomainSuiteTest"
	dd := domainDaoImpl{db: ds.tx}

	name, err := dd.Create(context.Background(), orgId)
	assert.NoError(ds.T(), err)
	assert.NotEmpty(ds.T(), name)

	name, err = dd.Fetch(context.Background(), orgId)
	assert.NoError(ds.T(), err)
	assert.NotEmpty(ds.T(), name)

	err = dd.Delete(context.Background(), orgId, name)
	assert.NoError(ds.T(), err)

	name, err = dd.Fetch(context.Background(), orgId)
	assert.NoError(ds.T(), err)
	assert.Empty(ds.T(), name)
}
