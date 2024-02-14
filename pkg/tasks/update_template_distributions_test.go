package tasks

import (
	"fmt"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type UpdateTemplateDistributionsSuite struct {
	suite.Suite
}

func TestUpdateTemplateDistributionsSuite(t *testing.T) {
	suite.Run(t, new(UpdateTemplateDistributionsSuite))
}

func (s *UpdateTemplateDistributionsSuite) TestGetDistributionPath() {
	repoUUID := "repo-uuid"
	templateUUID := "template-uuid"
	snapshotUUID := "snapshot-uuid"
	url := "http://example.com/red/hat/repo/path/"
	expectedRhPath := fmt.Sprintf("templates/%v/%v", templateUUID, "red/hat/repo/path")
	expectedCustomPath := fmt.Sprintf("templates/%v/%v", templateUUID, repoUUID)
	expectedName := templateUUID + "/" + snapshotUUID

	repo := api.RepositoryResponse{UUID: repoUUID, URL: url, OrgID: config.RedHatOrg}
	distPath, distName, err := getDistPathAndName(repo, templateUUID, snapshotUUID)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), expectedRhPath, distPath)
	assert.Equal(s.T(), expectedName, distName)

	repo = api.RepositoryResponse{UUID: repoUUID, URL: url, OrgID: "12345"}
	distPath, _, err = getDistPathAndName(repo, templateUUID, snapshotUUID)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), expectedCustomPath, distPath)
	assert.Equal(s.T(), expectedName, distName)
}
