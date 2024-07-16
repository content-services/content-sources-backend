package models

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type TemplateSuite struct {
	*ModelsSuite
}

func TestTemplateSuite(t *testing.T) {
	m := ModelsSuite{}
	r := TemplateSuite{&m}
	suite.Run(t, &r)
}

func (suite *TemplateSuite) TestCreateInvalidVersion() {
	var repoConfig = Template{
		Name:    "foo",
		OrgID:   "1",
		Version: "redhat linux 3.14",
		Arch:    "x86_64",
	}
	res := suite.tx.Create(&repoConfig)
	assert.NotNil(suite.T(), res.Error)
	assert.True(suite.T(), strings.Contains(res.Error.Error(), "version"))
}

func (suite *TemplateSuite) TestCreateInvalidArch() {
	var repoConfig = Template{
		Name:    "foo",
		OrgID:   "1",
		Arch:    "68000",
		Version: "8",
	}
	res := suite.tx.Create(&repoConfig)
	assert.Error(suite.T(), res.Error)
	assert.True(suite.T(), strings.Contains(res.Error.Error(), "arch"))
}

func (suite *TemplateSuite) TestCreateBlankVersion() {
	var repoConfig = Template{
		Name:    "foo",
		OrgID:   "1",
		Version: "",
		Arch:    "x86_64",
	}
	res := suite.tx.Create(&repoConfig)
	assert.NotNil(suite.T(), res.Error)
	assert.Equal(suite.T(), res.Error.Error(), "Version cannot be blank.")
}

func (suite *TemplateSuite) TestCreateBlankArch() {
	var repoConfig = Template{
		Name:    "foo",
		OrgID:   "1",
		Version: "8",
		Arch:    "",
	}
	res := suite.tx.Create(&repoConfig)
	assert.NotNil(suite.T(), res.Error)
	assert.Equal(suite.T(), res.Error.Error(), "Arch cannot be blank.")
}

func (suite *TemplateSuite) TestCreateBlankName() {
	var repoConfig = Template{
		Name:    "",
		OrgID:   "1",
		Version: "8",
		Arch:    "x86_64",
	}
	res := suite.tx.Create(&repoConfig)
	assert.NotNil(suite.T(), res.Error)
	assert.Equal(suite.T(), res.Error.Error(), "Name cannot be blank.")
}

func (suite *TemplateSuite) TestCreateBlankOrgID() {
	var repoConfig = Template{
		Name:    "foo",
		OrgID:   "",
		Version: "8",
		Arch:    "",
	}
	res := suite.tx.Create(&repoConfig)
	assert.NotNil(suite.T(), res.Error)
	assert.Equal(suite.T(), res.Error.Error(), "Org ID cannot be blank.")
}
