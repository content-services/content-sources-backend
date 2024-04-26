package dao

import (
	"context"
	"sync"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/db"
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

func TestConcurrentGetDomainName(t *testing.T) {
	// Note, this test does not use a transaction, as it fails when multiple go routines are trying to do that
	orgId := uuid2.NewString()
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			dDao := GetDomainDao(db.DB)
			dName, err := dDao.FetchOrCreateDomain(context.Background(), orgId)
			assert.NoError(t, err)
			assert.NotEmpty(t, dName)
			wg.Done()
		}()
	}
	wg.Wait()
}
