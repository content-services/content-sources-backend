package dao

import (
	"context"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/stretchr/testify/suite"
)

type LightwellAdvisorySuite struct {
	*DaoSuite
}

func TestLightwellAdvisorySuite(t *testing.T) {
	m := DaoSuite{}
	suite.Run(t, &LightwellAdvisorySuite{DaoSuite: &m})
}

func (s *LightwellAdvisorySuite) createLightwellRepoConfig(name string) string {
	repo := models.Repository{
		Origin:                  config.OriginLightwell,
		ContentType:             config.ContentTypeMaven,
		LastIntrospectionStatus: config.StatusValid,
	}
	err := s.tx.Create(&repo).Error
	s.Require().NoError(err)

	repoConfig := models.RepositoryConfiguration{
		Name:           name,
		OrgID:          config.LightwellOrg,
		RepositoryUUID: repo.UUID,
	}
	err = s.tx.Create(&repoConfig).Error
	s.Require().NoError(err)
	return repoConfig.UUID
}

func (s *LightwellAdvisorySuite) TestSyncInsertsNew() {
	dao := GetLightwellAdvisoryDao(s.tx)
	repoConfigUUID := s.createLightwellRepoConfig("lightwell/java/remediated")

	advisories := []LightwellAdvisoryInput{
		{
			AdvisoryID:    "FAKE-001",
			Severity:      "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:N/I:N/A:N",
			Details:       "Fake advisory one",
			ReferenceURLs: []string{"https://example.com/1"},
			PackageName:   "com.example:fake-lib",
			FixedVersion:  "1.0.1",
			Checksum:      "aaa111",
		},
		{
			AdvisoryID:    "FAKE-002",
			Severity:      "CVSS:3.1/AV:L/AC:H/PR:L/UI:N/S:U/C:H/I:H/A:H",
			Details:       "Fake advisory two",
			ReferenceURLs: []string{"https://example.com/2"},
			PackageName:   "com.example:other-lib",
			FixedVersion:  "2.0.0",
			Checksum:      "bbb222",
		},
	}

	err := dao.SyncForRepository(context.Background(), repoConfigUUID, "lightwell/java/remediated", advisories)
	s.NoError(err)

	result, err := dao.ListByRepository(context.Background(), repoConfigUUID)
	s.NoError(err)
	s.Len(result, 2)
}

func (s *LightwellAdvisorySuite) TestSyncUpdatesExisting() {
	dao := GetLightwellAdvisoryDao(s.tx)
	repoConfigUUID := s.createLightwellRepoConfig("lightwell/java/remediated")

	initial := []LightwellAdvisoryInput{
		{
			AdvisoryID:   "FAKE-001",
			Details:      "Original details",
			PackageName:  "com.example:fake-lib",
			FixedVersion: "1.0.0",
			Checksum:     "aaa111",
		},
	}
	err := dao.SyncForRepository(context.Background(), repoConfigUUID, "lightwell/java/remediated", initial)
	s.Require().NoError(err)

	updated := []LightwellAdvisoryInput{
		{
			AdvisoryID:   "FAKE-001",
			Details:      "Updated details",
			PackageName:  "com.example:fake-lib",
			FixedVersion: "1.0.1",
			Checksum:     "ccc333",
		},
	}
	err = dao.SyncForRepository(context.Background(), repoConfigUUID, "lightwell/java/remediated", updated)
	s.NoError(err)

	result, err := dao.ListByRepository(context.Background(), repoConfigUUID)
	s.NoError(err)
	s.Len(result, 1)
	s.Equal("Updated details", result[0].Details)
	s.Equal("1.0.1", result[0].FixedVersion)
	s.Equal("ccc333", result[0].Checksum)
}

func (s *LightwellAdvisorySuite) TestSyncDeletesStale() {
	dao := GetLightwellAdvisoryDao(s.tx)
	repoConfigUUID := s.createLightwellRepoConfig("lightwell/java/remediated")

	initial := []LightwellAdvisoryInput{
		{AdvisoryID: "FAKE-001", Checksum: "aaa"},
		{AdvisoryID: "FAKE-002", Checksum: "bbb"},
		{AdvisoryID: "FAKE-003", Checksum: "ccc"},
	}
	err := dao.SyncForRepository(context.Background(), repoConfigUUID, "lightwell/java/remediated", initial)
	s.Require().NoError(err)

	kept := []LightwellAdvisoryInput{
		{AdvisoryID: "FAKE-001", Checksum: "aaa"},
	}
	err = dao.SyncForRepository(context.Background(), repoConfigUUID, "lightwell/java/remediated", kept)
	s.NoError(err)

	result, err := dao.ListByRepository(context.Background(), repoConfigUUID)
	s.NoError(err)
	s.Len(result, 1)
	s.Equal("FAKE-001", result[0].AdvisoryID)
}

func (s *LightwellAdvisorySuite) TestListByRepository() {
	dao := GetLightwellAdvisoryDao(s.tx)
	repoConfigUUID1 := s.createLightwellRepoConfig("lightwell/java/remediated")
	repoConfigUUID2 := s.createLightwellRepoConfig("lightwell/java/other")

	err := dao.SyncForRepository(context.Background(), repoConfigUUID1, "repo-1", []LightwellAdvisoryInput{
		{AdvisoryID: "FAKE-001", Checksum: "aaa"},
		{AdvisoryID: "FAKE-002", Checksum: "bbb"},
	})
	s.Require().NoError(err)

	err = dao.SyncForRepository(context.Background(), repoConfigUUID2, "repo-2", []LightwellAdvisoryInput{
		{AdvisoryID: "FAKE-003", Checksum: "ccc"},
	})
	s.Require().NoError(err)

	result1, err := dao.ListByRepository(context.Background(), repoConfigUUID1)
	s.NoError(err)
	s.Len(result1, 2)

	result2, err := dao.ListByRepository(context.Background(), repoConfigUUID2)
	s.NoError(err)
	s.Len(result2, 1)
}
