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
	var repoConfig models.RepositoryConfiguration
	s.tx.Where("name = ? AND org_id = ?", name, config.LightwellOrg).First(&repoConfig)
	if repoConfig.UUID != "" {
		return repoConfig.UUID
	}

	repo := models.Repository{
		Origin:                  config.OriginLightwell,
		ContentType:             config.ContentTypeMaven,
		LastIntrospectionStatus: config.StatusValid,
	}
	err := s.tx.Create(&repo).Error
	s.Require().NoError(err)

	repoConfig = models.RepositoryConfiguration{
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
			FixedVersions: []string{"1.0.1"},
			Checksum:      "aaa111",
		},
		{
			AdvisoryID:    "FAKE-002",
			Severity:      "CVSS:3.1/AV:L/AC:H/PR:L/UI:N/S:U/C:H/I:H/A:H",
			Details:       "Fake advisory two",
			ReferenceURLs: []string{"https://example.com/2"},
			PackageName:   "com.example:other-lib",
			FixedVersions: []string{"2.0.0"},
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
			AdvisoryID:    "FAKE-001",
			Details:       "Original details",
			PackageName:   "com.example:fake-lib",
			FixedVersions: []string{"1.0.0"},
			Checksum:      "aaa111",
		},
	}
	err := dao.SyncForRepository(context.Background(), repoConfigUUID, "lightwell/java/remediated", initial)
	s.Require().NoError(err)

	updated := []LightwellAdvisoryInput{
		{
			AdvisoryID:    "FAKE-001",
			Details:       "Updated details",
			PackageName:   "com.example:fake-lib",
			FixedVersions: []string{"1.0.1"},
			Checksum:      "ccc333",
		},
	}
	err = dao.SyncForRepository(context.Background(), repoConfigUUID, "lightwell/java/remediated", updated)
	s.NoError(err)

	result, err := dao.ListByRepository(context.Background(), repoConfigUUID)
	s.NoError(err)
	s.Len(result, 1)
	s.Equal("Updated details", result[0].Details)
	s.Equal([]string{"1.0.1"}, result[0].FixedVersions)
	s.Equal("ccc333", result[0].Checksum)
}

func (s *LightwellAdvisorySuite) TestSyncDeletesStale() {
	dao := GetLightwellAdvisoryDao(s.tx)
	repoConfigUUID := s.createLightwellRepoConfig("lightwell/java/remediated")

	initial := []LightwellAdvisoryInput{
		{AdvisoryID: "FAKE-001", Checksum: "aaa", FixedVersions: []string{"1.0.0"}},
		{AdvisoryID: "FAKE-002", Checksum: "bbb", FixedVersions: []string{"1.0.0"}},
		{AdvisoryID: "FAKE-003", Checksum: "ccc", FixedVersions: []string{"1.0.0"}},
	}
	err := dao.SyncForRepository(context.Background(), repoConfigUUID, "lightwell/java/remediated", initial)
	s.Require().NoError(err)

	kept := []LightwellAdvisoryInput{
		{AdvisoryID: "FAKE-001", Checksum: "aaa", FixedVersions: []string{"1.0.0"}},
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
		{AdvisoryID: "FAKE-001", Checksum: "aaa", FixedVersions: []string{"1.0.0"}},
		{AdvisoryID: "FAKE-002", Checksum: "bbb", FixedVersions: []string{"1.0.0"}},
	})
	s.Require().NoError(err)

	err = dao.SyncForRepository(context.Background(), repoConfigUUID2, "repo-2", []LightwellAdvisoryInput{
		{AdvisoryID: "FAKE-003", Checksum: "ccc", FixedVersions: []string{"1.0.0"}},
	})
	s.Require().NoError(err)

	result1, err := dao.ListByRepository(context.Background(), repoConfigUUID1)
	s.NoError(err)
	s.Len(result1, 2)

	result2, err := dao.ListByRepository(context.Background(), repoConfigUUID2)
	s.NoError(err)
	s.Len(result2, 1)
}

func (s *LightwellAdvisorySuite) TestListUnnotifiedAdvisories() {
	dao := GetLightwellAdvisoryDao(s.tx)
	repoConfigUUID := s.createLightwellRepoConfig("lightwell/notif/list-all")

	advisories := []LightwellAdvisoryInput{
		{AdvisoryID: "CVE-2026-0001", PackageName: "com.example:lib-a", Severity: "9.8", FixedVersions: []string{"1.0.1"}, Checksum: "aaa"},
		{AdvisoryID: "CVE-2026-0002", PackageName: "com.example:lib-a", Severity: "7.5", FixedVersions: []string{"2.0.0"}, Checksum: "bbb"},
		{AdvisoryID: "CVE-2026-0003", PackageName: "com.example:lib-b", Severity: "4.0", FixedVersions: []string{"3.0.0"}, Checksum: "ccc"},
	}
	err := dao.SyncForRepository(context.Background(), repoConfigUUID, "lightwell/notif/list-all", advisories)
	s.Require().NoError(err)

	unnotified, err := dao.ListUnnotifiedAdvisories(context.Background(), repoConfigUUID)
	s.NoError(err)
	s.Len(unnotified, 3)
}

func (s *LightwellAdvisorySuite) TestListUnnotifiedAdvisoriesExcludesNotified() {
	dao := GetLightwellAdvisoryDao(s.tx)
	repoConfigUUID := s.createLightwellRepoConfig("lightwell/notif/excludes")

	advisories := []LightwellAdvisoryInput{
		{AdvisoryID: "CVE-2026-0001", PackageName: "com.example:lib-a", Severity: "9.8", FixedVersions: []string{"1.0.1"}, Checksum: "aaa"},
		{AdvisoryID: "CVE-2026-0002", PackageName: "com.example:lib-a", Severity: "7.5", FixedVersions: []string{"2.0.0"}, Checksum: "bbb"},
	}
	err := dao.SyncForRepository(context.Background(), repoConfigUUID, "lightwell/notif/excludes", advisories)
	s.Require().NoError(err)

	unnotified, err := dao.ListUnnotifiedAdvisories(context.Background(), repoConfigUUID)
	s.Require().NoError(err)
	s.Len(unnotified, 2)

	err = dao.MarkAsNotified(context.Background(), repoConfigUUID, unnotified[:1])
	s.Require().NoError(err)

	unnotified, err = dao.ListUnnotifiedAdvisories(context.Background(), repoConfigUUID)
	s.NoError(err)
	s.Len(unnotified, 1)
	s.Equal("CVE-2026-0002", unnotified[0].AdvisoryID)
}

func (s *LightwellAdvisorySuite) TestMarkAsNotifiedIdempotent() {
	dao := GetLightwellAdvisoryDao(s.tx)
	repoConfigUUID := s.createLightwellRepoConfig("lightwell/notif/idempotent")

	advisories := []LightwellAdvisoryInput{
		{AdvisoryID: "CVE-2026-0001", PackageName: "com.example:lib-a", Checksum: "aaa", FixedVersions: []string{"1.0.0"}},
	}
	err := dao.SyncForRepository(context.Background(), repoConfigUUID, "lightwell/notif/idempotent", advisories)
	s.Require().NoError(err)

	unnotified, err := dao.ListUnnotifiedAdvisories(context.Background(), repoConfigUUID)
	s.Require().NoError(err)

	err = dao.MarkAsNotified(context.Background(), repoConfigUUID, unnotified)
	s.NoError(err)

	err = dao.MarkAsNotified(context.Background(), repoConfigUUID, unnotified)
	s.NoError(err)

	result, err := dao.ListUnnotifiedAdvisories(context.Background(), repoConfigUUID)
	s.NoError(err)
	s.Empty(result)
}

func (s *LightwellAdvisorySuite) TestMarkAsNotifiedEmpty() {
	dao := GetLightwellAdvisoryDao(s.tx)
	err := dao.MarkAsNotified(context.Background(), "some-uuid", nil)
	s.NoError(err)
}

func (s *LightwellAdvisorySuite) TestListUnnotifiedAdvisoriesFixedVersions() {
	dao := GetLightwellAdvisoryDao(s.tx)
	repoConfigUUID := s.createLightwellRepoConfig("lightwell/notif/versions")

	advisories := []LightwellAdvisoryInput{
		{AdvisoryID: "CVE-2026-0001", PackageName: "com.example:lib-a", Severity: "9.8", FixedVersions: []string{"1.0.1"}, Checksum: "aaa"},
	}
	err := dao.SyncForRepository(context.Background(), repoConfigUUID, "lightwell/notif/versions", advisories)
	s.Require().NoError(err)

	unnotified, err := dao.ListUnnotifiedAdvisories(context.Background(), repoConfigUUID)
	s.NoError(err)
	s.Require().Len(unnotified, 1)
	s.Equal("CVE-2026-0001", unnotified[0].AdvisoryID)
	s.Equal("com.example:lib-a", unnotified[0].PackageName)
	s.Equal("9.8", unnotified[0].Severity)
	s.Equal([]string{"1.0.1"}, unnotified[0].FixedVersions)
}

func (s *LightwellAdvisorySuite) TestListUnnotifiedAdvisoriesMultipleFixedVersions() {
	dao := GetLightwellAdvisoryDao(s.tx)
	repoConfigUUID := s.createLightwellRepoConfig("lightwell/notif/multi-versions")

	advisories := []LightwellAdvisoryInput{
		{AdvisoryID: "CVE-2026-0001", PackageName: "com.example:lib-a", Severity: "9.8", FixedVersions: []string{"1.0.1", "2.0.0"}, Checksum: "aaa"},
	}
	err := dao.SyncForRepository(context.Background(), repoConfigUUID, "lightwell/notif/multi-versions", advisories)
	s.Require().NoError(err)

	unnotified, err := dao.ListUnnotifiedAdvisories(context.Background(), repoConfigUUID)
	s.NoError(err)
	s.Require().Len(unnotified, 1)
	s.Equal([]string{"1.0.1", "2.0.0"}, unnotified[0].FixedVersions)
}
