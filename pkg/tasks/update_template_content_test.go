package tasks

import (
	"fmt"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type UpdateTemplateContentSuite struct {
	suite.Suite
}

func TestUpdateTemplateContentSuiteSuite(t *testing.T) {
	suite.Run(t, new(UpdateTemplateContentSuite))
}

func (s *UpdateTemplateContentSuite) TestGetDistributionPath() {
	repoUUID := "repo-uuid"
	templateUUID := "template-uuid"
	url := "http://example.com/red/hat/repo/path/"
	expectedRhPath := fmt.Sprintf("templates/%v/%v", templateUUID, "red/hat/repo/path")
	expectedCustomPath := fmt.Sprintf("templates/%v/%v", templateUUID, repoUUID)
	expectedName := templateUUID + "/" + repoUUID

	repo := api.RepositoryResponse{UUID: repoUUID, URL: url, OrgID: config.RedHatOrg}
	distPath, distName, err := getDistPathAndName(repo, templateUUID)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), expectedRhPath, distPath)
	assert.Equal(s.T(), expectedName, distName)

	repo = api.RepositoryResponse{UUID: repoUUID, URL: url, OrgID: "12345"}
	distPath, _, err = getDistPathAndName(repo, templateUUID)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), expectedCustomPath, distPath)
	assert.Equal(s.T(), expectedName, distName)
}
