package tasks

import (
	"strings"
	"testing"

	caliri "github.com/content-services/caliri/release/v4"
	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type CandlepinHelpersTest struct {
	suite.Suite
}

func TestCandlepinHelpersSuite(t *testing.T) {
	suite.Run(t, new(CandlepinHelpersTest))
}

func (s *CandlepinHelpersTest) TestGenContentDto() {
	repo := api.RepositoryResponse{
		UUID:      "abce",
		Name:      "MyTestRepo",
		Label:     "my_test_repo",
		URL:       "http://example.com/repo",
		Origin:    config.OriginExternal,
		AccountID: "1234",
		OrgID:     "1234",
		GpgKey:    "",
	}

	dto := GenContentDto(repo)
	assert.Equal(s.T(), repo.Name, *dto.Name)
	assert.Equal(s.T(), repo.Label, *dto.Label)
	assert.Equal(s.T(), "", *dto.GpgUrl)

	repo.GpgKey = "REAL SECURE"
	dto = GenContentDto(repo)
	assert.NotEqual(s.T(), "", *dto.GpgUrl)
	assert.True(s.T(), strings.HasSuffix(*dto.GpgUrl, "/api/content-sources/v1.0/repository_gpg_key/abce"))
}

func (s *CandlepinHelpersTest) TestGenContentDtoUpload() {
	repo := api.RepositoryResponse{
		UUID:              "abce",
		Name:              "MyTestRepo",
		Label:             "my_test_repo",
		URL:               "",
		Origin:            config.OriginUpload,
		AccountID:         "1234",
		OrgID:             "1234",
		GpgKey:            "",
		LatestSnapshotURL: "http://example.com/snapshot",
	}

	dto := GenContentDto(repo)
	assert.Equal(s.T(), repo.Name, *dto.Name)
	assert.Equal(s.T(), repo.Label, *dto.Label)
	assert.Equal(s.T(), "", *dto.GpgUrl)
	assert.Equal(s.T(), *dto.ContentUrl, repo.LatestSnapshotURL)
}

func (s *CandlepinHelpersTest) TestUnneededOverrides() {
	existing := []caliri.ContentOverrideDTO{{
		Name:         utils.Ptr("foo"),
		ContentLabel: utils.Ptr("label1"),
		Value:        utils.Ptr("3"),
	}, {
		Name:         utils.Ptr("foo2"),
		ContentLabel: utils.Ptr("label2"),
		Value:        utils.Ptr("4"),
	}}

	expected := []caliri.ContentOverrideDTO{{
		Name:         utils.Ptr("foo"),
		ContentLabel: utils.Ptr("label1"),
		Value:        utils.Ptr("3"),
	}}

	toRemove := UnneededOverrides(existing, expected)
	assert.Len(s.T(), toRemove, 1)
	assert.Equal(s.T(), existing[1], toRemove[0])

	toRemove = UnneededOverrides(existing, existing)
	assert.Len(s.T(), toRemove, 0)
}

func (s *CandlepinHelpersTest) TestContentOverridesForRepo() {
	//	func ContentOverridesForRepo(orgId string, domainName string, templateUUID string, pulpContentPath string, repo api.RepositoryResponse) ([]caliri.ContentOverrideDTO, error) {
	orgId := "myorg"
	domain := "myDomain"
	templateUUID := "abcdef"
	pulpContentPath := "/pulp"
	Repo := api.RepositoryResponse{
		OrgID:          orgId,
		UUID:           "xyz",
		Name:           "mycustomRepo",
		Label:          "mylabel",
		URL:            "http://example.com/upstream",
		GpgKey:         "",
		ModuleHotfixes: false,
		LastSnapshot:   utils.Ptr(api.SnapshotResponse{}),
	}
	overrides, err := ContentOverridesForRepo(orgId, domain, templateUUID, pulpContentPath, Repo)
	assert.NoError(s.T(), err)

	assert.Len(s.T(), overrides, 3)
	assert.Contains(s.T(), overrides, caliri.ContentOverrideDTO{
		Name:         utils.Ptr("sslcacert"),
		ContentLabel: &Repo.Label,
		Value:        utils.Ptr(" "),
	})
	assert.Contains(s.T(), overrides, caliri.ContentOverrideDTO{
		Name:         utils.Ptr("sslverifystatus"),
		ContentLabel: &Repo.Label,
		Value:        utils.Ptr("0"),
	})
	assert.Contains(s.T(), overrides, caliri.ContentOverrideDTO{
		Name:         utils.Ptr("baseurl"),
		ContentLabel: utils.Ptr(Repo.Label),
		Value:        utils.Ptr("/pulp/myDomain/templates/abcdef/xyz"),
	})

	Repo.ModuleHotfixes = true
	overrides, err = ContentOverridesForRepo(orgId, domain, templateUUID, pulpContentPath, Repo)
	assert.NoError(s.T(), err)
	assert.Len(s.T(), overrides, 4)
	assert.Contains(s.T(), overrides, caliri.ContentOverrideDTO{
		Name:         utils.Ptr("module_hotfixes"),
		ContentLabel: &Repo.Label,
		Value:        utils.Ptr("1"),
	})
	Repo.OrgID = config.RedHatOrg
	Repo.Origin = config.OriginRedHat
	overrides, err = ContentOverridesForRepo(orgId, domain, templateUUID, pulpContentPath, Repo)
	assert.NoError(s.T(), err)
	assert.Len(s.T(), overrides, 2)
	assert.Contains(s.T(), overrides, caliri.ContentOverrideDTO{
		Name:         utils.Ptr("sslcacert"),
		ContentLabel: &Repo.Label,
		Value:        utils.Ptr(" "),
	})
	assert.Contains(s.T(), overrides, caliri.ContentOverrideDTO{
		Name:         utils.Ptr("sslverifystatus"),
		ContentLabel: &Repo.Label,
		Value:        utils.Ptr("0"),
	})
}
