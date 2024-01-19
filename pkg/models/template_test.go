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
	}
	res := suite.tx.Create(&repoConfig)
	assert.NotNil(suite.T(), res.Error)
	assert.True(suite.T(), strings.Contains(res.Error.Error(), "version"))
}

func (suite *TemplateSuite) TestCreateInvalidArch() {
	var repoConfig = Template{
		Name:  "foo",
		OrgID: "1",
		Arch:  "68000",
	}
	res := suite.tx.Create(&repoConfig)
	assert.Error(suite.T(), res.Error)
	assert.True(suite.T(), strings.Contains(res.Error.Error(), "arch"))
}
