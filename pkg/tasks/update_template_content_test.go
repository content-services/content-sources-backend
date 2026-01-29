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

func (s *UpdateTemplateContentSuite) TestNormalizeExtendedReleaseLabel() {
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "e4s label with minor version",
			input:    "rhel-8.6-for-x86_64-appstream-e4s-rpms",
			expected: "rhel-8-for-x86_64-appstream-e4s-rpms",
		},
		{
			name:     "eus label with minor version",
			input:    "rhel-9.4-for-x86_64-appstream-eus-rpms",
			expected: "rhel-9-for-x86_64-appstream-eus-rpms",
		},
		{
			name:     "regular rhel label without extended release",
			input:    "rhel-8-for-x86_64-appstream-rpms",
			expected: "rhel-8-for-x86_64-appstream-rpms",
		},
		{
			name:     "already normalized e4s label",
			input:    "rhel-9-for-x86_64-appstream-e4s-rpms",
			expected: "rhel-9-for-x86_64-appstream-e4s-rpms",
		},
		{
			name:     "custom repository label",
			input:    "custom-repo-label",
			expected: "custom-repo-label",
		},
	}

	for _, tc := range testCases {
		s.T().Run(tc.name, func(t *testing.T) {
			result := normalizeExtendedReleaseLabel(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}
