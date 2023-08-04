package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type DomainSuite struct {
	*ModelsSuite
}

func TestDomainSuite(t *testing.T) {
	m := ModelsSuite{}
	r := RpmSuite{&m}
	suite.Run(t, &r)
}

func (s *RpmSuite) TestDomainCreate() {
	t := s.T()
	tx := s.tx

	d := Domain{
		DomainName: "foo",
		OrgId:      "org",
	}

	err := tx.Create(d).Error
	assert.NoError(t, err)

	found := Domain{}

	err = tx.Model(&found).Where("domain_name = 'foo'").First(&found).Error
	assert.Nil(t, err)
	assert.Equal(t, d.OrgId, found.OrgId)
	assert.Equal(t, d.DomainName, found.DomainName)
}
