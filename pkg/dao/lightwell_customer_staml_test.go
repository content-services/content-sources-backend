package dao

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/db"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type LightwellCustomerStamlDaoSuite struct {
	suite.Suite
}

func TestLightwellCustomerStamlDaoSuite(t *testing.T) {
	suite.Run(t, new(LightwellCustomerStamlDaoSuite))
}

func (s *LightwellCustomerStamlDaoSuite) dao() (context.Context, LightwellCustomerStamlDao) {
	s.Require().NotNil(db.LightwellQueries)
	return context.Background(), GetDaoRegistry(db.DB).LightwellCustomerStaml
}

func (s *LightwellCustomerStamlDaoSuite) uniquePair() (string, string) {
	n := time.Now().UnixNano()
	return fmt.Sprintf("cid-%d", n), fmt.Sprintf("staml-%d", n)
}

func (s *LightwellCustomerStamlDaoSuite) TestCreateAndDelete() {
	ctx, daoImpl := s.dao()
	customerID, staml := s.uniquePair()

	created, err := daoImpl.Create(ctx, customerID, staml)
	s.Require().NoError(err)
	assert.Equal(s.T(), customerID, created.CustomerID)
	assert.Equal(s.T(), staml, created.Staml)
	assert.False(s.T(), created.CreatedAt.IsZero())

	s.T().Cleanup(func() {
		_ = daoImpl.Delete(ctx, customerID, staml)
	})

	err = daoImpl.Delete(ctx, customerID, staml)
	s.Require().NoError(err)

	err = daoImpl.Delete(ctx, customerID, staml)
	s.Require().Error(err)
	var daoErr *ce.DaoError
	s.Require().ErrorAs(err, &daoErr)
	assert.True(s.T(), daoErr.NotFound)
}

func (s *LightwellCustomerStamlDaoSuite) TestCreateAlreadyExists() {
	ctx, daoImpl := s.dao()
	customerID, staml := s.uniquePair()

	_, err := daoImpl.Create(ctx, customerID, staml)
	s.Require().NoError(err)
	s.T().Cleanup(func() {
		_ = daoImpl.Delete(ctx, customerID, staml)
	})

	_, err = daoImpl.Create(ctx, customerID, staml)
	s.Require().Error(err)
	var daoErr *ce.DaoError
	s.Require().ErrorAs(err, &daoErr)
	assert.True(s.T(), daoErr.AlreadyExists)
}

func (s *LightwellCustomerStamlDaoSuite) TestCreateManyToMany() {
	ctx, daoImpl := s.dao()
	n := time.Now().UnixNano()
	cid1 := fmt.Sprintf("cid-m2m-a-%d", n)
	cid2 := fmt.Sprintf("cid-m2m-b-%d", n)
	staml1 := fmt.Sprintf("staml-m2m-a-%d", n)
	staml2 := fmt.Sprintf("staml-m2m-b-%d", n)

	pairs := [][2]string{
		{cid1, staml1},
		{cid1, staml2},
		{cid2, staml2},
	}
	for _, pair := range pairs {
		_, err := daoImpl.Create(ctx, pair[0], pair[1])
		s.Require().NoError(err)
	}
	s.T().Cleanup(func() {
		for _, pair := range pairs {
			_ = daoImpl.Delete(ctx, pair[0], pair[1])
		}
	})
}

func (s *LightwellCustomerStamlDaoSuite) TestListCustomerIds() {
	ctx, daoImpl := s.dao()
	n := time.Now().UnixNano()
	cid1 := fmt.Sprintf("cid-list-a-%d", n)
	cid2 := fmt.Sprintf("cid-list-b-%d", n)
	cidOther := fmt.Sprintf("cid-list-c-%d", n)
	staml := fmt.Sprintf("staml-list-%d", n)
	other := fmt.Sprintf("staml-list-other-%d", n)

	pairs := [][2]string{
		{cid1, staml},
		{cid2, staml},
		{cidOther, other},
	}
	for _, pair := range pairs {
		_, err := daoImpl.Create(ctx, pair[0], pair[1])
		s.Require().NoError(err)
	}
	s.T().Cleanup(func() {
		for _, pair := range pairs {
			_ = daoImpl.Delete(ctx, pair[0], pair[1])
		}
	})

	ids, err := daoImpl.ListCustomerIds(ctx, staml)
	s.Require().NoError(err)
	assert.Equal(s.T(), []string{cid1, cid2}, ids)

	ids, err = daoImpl.ListCustomerIds(ctx, "nobody")
	s.Require().NoError(err)
	assert.Empty(s.T(), ids)
}

func (s *LightwellCustomerStamlDaoSuite) TestHasAccess() {
	ctx, daoImpl := s.dao()
	customerID, staml := s.uniquePair()

	ok, err := daoImpl.HasAccess(ctx, customerID, staml)
	s.Require().NoError(err)
	assert.False(s.T(), ok)

	_, err = daoImpl.Create(ctx, customerID, staml)
	s.Require().NoError(err)
	s.T().Cleanup(func() {
		_ = daoImpl.Delete(ctx, customerID, staml)
	})

	ok, err = daoImpl.HasAccess(ctx, customerID, staml)
	s.Require().NoError(err)
	assert.True(s.T(), ok)

	ok, err = daoImpl.HasAccess(ctx, customerID, "other-staml")
	s.Require().NoError(err)
	assert.False(s.T(), ok)
}
