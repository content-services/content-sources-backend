package dao

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/db"
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
	return s.createLightwellRepoConfigWithFeature(name, "lightwell-network")
}

func (s *LightwellAdvisorySuite) createLightwellRepoConfigWithFeature(name string, featureName string) string {
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
		FeatureName:    featureName,
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

	published := time.Date(2026, 9, 17, 18, 39, 42, 0, time.UTC)
	initial := []LightwellAdvisoryInput{
		{
			AdvisoryID:     "FAKE-001",
			Details:        "Original details",
			PackageName:    "com.example:fake-lib",
			PackageVersion: "1.0.0",
			FixedVersions:  []string{"1.0.0"},
			Checksum:       "aaa111",
			Summary:        "Original summary",
			Source:         "pnc-build",
			SchemaVersion:  "1.6.8",
			Aliases:        []string{"FAKE-001"},
			Published:      &published,
			Modified:       &published,
		},
	}
	err := dao.SyncForRepository(context.Background(), repoConfigUUID, "lightwell/java/remediated", initial)
	s.Require().NoError(err)

	updatedPublished := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	updated := []LightwellAdvisoryInput{
		{
			AdvisoryID:     "FAKE-001",
			Details:        "Updated details",
			PackageName:    "com.example:fake-lib",
			PackageVersion: "1.0.1",
			FixedVersions:  []string{"1.0.1"},
			Checksum:       "ccc333",
			Summary:        "Updated summary",
			Source:         "manual",
			SchemaVersion:  "1.7.0",
			Aliases:        []string{"FAKE-001", "CVE-2099-0001"},
			Published:      &updatedPublished,
			Modified:       &updatedPublished,
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
	s.Equal("1.0.1", result[0].PackageVersion)
	s.Equal("Updated summary", result[0].Summary)
	s.Equal("manual", result[0].Source)
	s.Equal("1.7.0", result[0].SchemaVersion)
	s.Equal([]string{"FAKE-001", "CVE-2099-0001"}, []string(result[0].Aliases))
	s.Require().NotNil(result[0].Published)
	s.True(updatedPublished.Equal(*result[0].Published))
}

func (s *LightwellAdvisorySuite) TestSyncPersistsOSVMetadata() {
	dao := GetLightwellAdvisoryDao(s.tx)
	repoConfigUUID := s.createLightwellRepoConfig("lightwell/java/osv-meta")

	published := time.Date(2026, 9, 17, 18, 39, 42, 0, time.UTC)
	modified := published
	advisories := []LightwellAdvisoryInput{
		{
			AdvisoryID:     "x_RHLW-CVE-2015-6748-1.7.2",
			Severity:       "6.1",
			Details:        "Cross-site scripting (XSS) vulnerability in jsoup before 1.8.3.",
			PackageName:    "org.jsoup:jsoup",
			PackageVersion: "1.7.2",
			FixedVersions:  []string{"1.7.2.rhlw-00001"},
			Checksum:       "abc123",
			Published:      &published,
			Modified:       &modified,
			Aliases:        []string{"GHSA-48rh-qgjr-xfj6", "CVE-2015-6748"},
			SchemaVersion:  "1.6.8",
			Source:         "pnc-build",
			Summary:        "Improper Neutralization of Input During Web Page Generation in Jsoup",
		},
	}
	err := dao.SyncForRepository(context.Background(), repoConfigUUID, "lightwell/java/remediated", advisories)
	s.Require().NoError(err)

	result, err := dao.ListByRepository(context.Background(), repoConfigUUID)
	s.NoError(err)
	s.Require().Len(result, 1)
	got := result[0]
	s.Equal("1.7.2", got.PackageVersion)
	s.Equal("pnc-build", got.Source)
	s.Equal("1.6.8", got.SchemaVersion)
	s.Equal("Improper Neutralization of Input During Web Page Generation in Jsoup", got.Summary)
	s.Equal([]string{"GHSA-48rh-qgjr-xfj6", "CVE-2015-6748"}, []string(got.Aliases))
	s.Require().NotNil(got.Published)
	s.True(published.Equal(*got.Published))
}

func (s *LightwellAdvisorySuite) TestSyncWritesAdvisoryReleases() {
	dao := GetLightwellAdvisoryDao(s.tx)
	repoConfigUUID := s.createLightwellRepoConfig("lightwell/java/releases")

	err := dao.SyncForRepository(context.Background(), repoConfigUUID, "lightwell/java/releases", []LightwellAdvisoryInput{
		{
			AdvisoryID:     "x_RHLW-CVE-2015-6748-1.2.3",
			PackageName:    "org.jsoup:jsoup",
			PackageVersion: "1.2.3",
			FixedVersions:  []string{"1.2.3.rhlw.00003.n00001.hf00001"},
			Checksum:       "rel-1",
		},
		{
			AdvisoryID:    "FAKE-UPSTREAM",
			PackageName:   "com.example:other",
			FixedVersions: []string{"1.0.1"},
			Checksum:      "rel-2",
		},
	})
	s.Require().NoError(err)

	var parent models.LightwellAdvisory
	s.Require().NoError(s.tx.Where("advisory_id = ?", "x_RHLW-CVE-2015-6748-1.2.3").First(&parent).Error)

	var releases []models.LightwellAdvisoryRelease
	s.Require().NoError(s.tx.Where("advisory_uuid = ?", parent.UUID).Find(&releases).Error)
	s.Require().Len(releases, 1)
	s.Equal("1.2.3.rhlw.00003.n00001.hf00001", releases[0].ReleaseVersion)
	s.Equal(3, releases[0].RhlwBaseline)
	s.Equal(1, releases[0].RhlwNovel)
	s.Equal(1, releases[0].RhlwHotfix)

	var other models.LightwellAdvisory
	s.Require().NoError(s.tx.Where("advisory_id = ?", "FAKE-UPSTREAM").First(&other).Error)
	s.Require().NoError(s.tx.Where("advisory_uuid = ?", other.UUID).Find(&releases).Error)
	s.Empty(releases)

	err = dao.SyncForRepository(context.Background(), repoConfigUUID, "lightwell/java/releases", []LightwellAdvisoryInput{
		{
			AdvisoryID:     "x_RHLW-CVE-2015-6748-1.2.3",
			PackageName:    "org.jsoup:jsoup",
			PackageVersion: "1.2.3",
			FixedVersions:  []string{"1.2.3.rhlw.00004"},
			Checksum:       "rel-1-updated",
		},
	})
	s.Require().NoError(err)

	s.Require().NoError(s.tx.Where("advisory_id = ?", "x_RHLW-CVE-2015-6748-1.2.3").First(&parent).Error)
	s.Require().NoError(s.tx.Where("advisory_uuid = ?", parent.UUID).Find(&releases).Error)
	s.Require().Len(releases, 1)
	s.Equal("1.2.3.rhlw.00004", releases[0].ReleaseVersion)
	s.Equal(4, releases[0].RhlwBaseline)
	s.Equal(0, releases[0].RhlwNovel)
	s.Equal(0, releases[0].RhlwHotfix)

	s.Error(s.tx.Where("advisory_id = ?", "FAKE-UPSTREAM").First(&other).Error)
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

func (s *LightwellAdvisorySuite) TestList() {
	dao := GetLightwellAdvisoryDao(s.tx)
	javaUUID := s.createLightwellRepoConfig("lightwell/java/remediated")
	pythonUUID := s.createLightwellRepoConfig("lightwell/python/validated")

	err := dao.SyncForRepository(context.Background(), javaUUID, "lightwell/java/remediated", []LightwellAdvisoryInput{
		{
			AdvisoryID:    "x_DEMO-CVE-0000-0001-1.2.3",
			PackageName:   "com.example:demo-lib",
			FixedVersions: []string{"1.2.3.build-00001"},
			Checksum:      "aaa111",
		},
	})
	s.Require().NoError(err)
	err = dao.SyncForRepository(context.Background(), pythonUUID, "lightwell/python/validated", []LightwellAdvisoryInput{
		{
			AdvisoryID:    "x_DEMO-CVE-0000-0002-4.0.0",
			PackageName:   "demo-pkg",
			FixedVersions: []string{"4.0.0"},
			Checksum:      "bbb222",
		},
	})
	s.Require().NoError(err)

	result, total, err := dao.List(context.Background(), 0, 10)
	s.NoError(err)
	s.Equal(int64(2), total)
	s.Require().Len(result, 2)
	byID := map[string]LightwellAdvisoryInput{}
	for _, row := range result {
		byID[row.AdvisoryID] = row
	}
	s.Equal("lightwell/java/remediated", byID["x_DEMO-CVE-0000-0001-1.2.3"].RepoName)
	s.Equal("com.example:demo-lib", byID["x_DEMO-CVE-0000-0001-1.2.3"].PackageName)
	s.Equal("lightwell/python/validated", byID["x_DEMO-CVE-0000-0002-4.0.0"].RepoName)

	page, total, err := dao.List(context.Background(), 0, 1)
	s.NoError(err)
	s.Equal(int64(2), total)
	s.Len(page, 1)
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

	unnotified, err := dao.ListUnnotifiedAdvisories(context.Background(), repoConfigUUID, "org-1")
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

	unnotified, err := dao.ListUnnotifiedAdvisories(context.Background(), repoConfigUUID, "org-1")
	s.Require().NoError(err)
	s.Len(unnotified, 2)

	err = dao.MarkAsNotified(context.Background(), repoConfigUUID, "org-1", unnotified[:1])
	s.Require().NoError(err)

	unnotified, err = dao.ListUnnotifiedAdvisories(context.Background(), repoConfigUUID, "org-1")
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

	unnotified, err := dao.ListUnnotifiedAdvisories(context.Background(), repoConfigUUID, "org-1")
	s.Require().NoError(err)

	err = dao.MarkAsNotified(context.Background(), repoConfigUUID, "org-1", unnotified)
	s.NoError(err)

	err = dao.MarkAsNotified(context.Background(), repoConfigUUID, "org-1", unnotified)
	s.NoError(err)

	result, err := dao.ListUnnotifiedAdvisories(context.Background(), repoConfigUUID, "org-1")
	s.NoError(err)
	s.Empty(result)
}

func (s *LightwellAdvisorySuite) TestMarkAsNotifiedEmpty() {
	dao := GetLightwellAdvisoryDao(s.tx)
	err := dao.MarkAsNotified(context.Background(), "some-uuid", "org-1", nil)
	s.NoError(err)
}

func (s *LightwellAdvisorySuite) TestListUnnotifiedAdvisoriesPerOrg() {
	dao := GetLightwellAdvisoryDao(s.tx)
	repoConfigUUID := s.createLightwellRepoConfig("lightwell/notif/per-org")

	advisories := []LightwellAdvisoryInput{
		{AdvisoryID: "CVE-2026-0001", PackageName: "com.example:lib-a", Severity: "9.8", FixedVersions: []string{"1.0.1"}, Checksum: "aaa"},
	}
	err := dao.SyncForRepository(context.Background(), repoConfigUUID, "lightwell/notif/per-org", advisories)
	s.Require().NoError(err)

	unnotifiedOrg1, err := dao.ListUnnotifiedAdvisories(context.Background(), repoConfigUUID, "org-1")
	s.Require().NoError(err)
	s.Len(unnotifiedOrg1, 1)

	err = dao.MarkAsNotified(context.Background(), repoConfigUUID, "org-1", unnotifiedOrg1)
	s.Require().NoError(err)

	unnotifiedOrg1, err = dao.ListUnnotifiedAdvisories(context.Background(), repoConfigUUID, "org-1")
	s.NoError(err)
	s.Empty(unnotifiedOrg1)

	unnotifiedOrg2, err := dao.ListUnnotifiedAdvisories(context.Background(), repoConfigUUID, "org-2")
	s.NoError(err)
	s.Len(unnotifiedOrg2, 1)
}

func (s *LightwellAdvisorySuite) TestListUnnotifiedAdvisoriesFixedVersions() {
	dao := GetLightwellAdvisoryDao(s.tx)
	repoConfigUUID := s.createLightwellRepoConfig("lightwell/notif/versions")

	advisories := []LightwellAdvisoryInput{
		{AdvisoryID: "CVE-2026-0001", PackageName: "com.example:lib-a", Severity: "9.8", FixedVersions: []string{"1.0.1"}, Checksum: "aaa"},
	}
	err := dao.SyncForRepository(context.Background(), repoConfigUUID, "lightwell/notif/versions", advisories)
	s.Require().NoError(err)

	unnotified, err := dao.ListUnnotifiedAdvisories(context.Background(), repoConfigUUID, "org-1")
	s.NoError(err)
	s.Require().Len(unnotified, 1)
	s.Equal("CVE-2026-0001", unnotified[0].AdvisoryID)
	s.Equal("com.example:lib-a", unnotified[0].PackageName)
	s.Equal("9.8", unnotified[0].Severity)
	s.Equal([]string{"1.0.1"}, unnotified[0].FixedVersions)
}

func (s *LightwellAdvisorySuite) createCommittedLightwellRepoConfig(name, featureName string) string {
	repo := models.Repository{
		Origin:                  config.OriginLightwell,
		ContentType:             config.ContentTypeMaven,
		LastIntrospectionStatus: config.StatusValid,
	}
	err := db.DB.Create(&repo).Error
	s.Require().NoError(err)

	repoConfig := models.RepositoryConfiguration{
		Name:           name,
		OrgID:          config.LightwellOrg,
		RepositoryUUID: repo.UUID,
		FeatureName:    featureName,
	}
	err = db.DB.Create(&repoConfig).Error
	s.Require().NoError(err)

	s.T().Cleanup(func() {
		_ = db.DB.Where("repository_configuration_uuid = ?", repoConfig.UUID).Delete(&models.LightwellAdvisory{}).Error
		_ = db.DB.Where("uuid = ?", repoConfig.UUID).Delete(&models.RepositoryConfiguration{}).Error
		_ = db.DB.Where("uuid = ?", repo.UUID).Delete(&models.Repository{}).Error
	})
	return repoConfig.UUID
}

func (s *LightwellAdvisorySuite) TestListAdvisoriesMapsOSVMetadata() {
	s.Require().NotNil(db.LightwellQueries)
	daoImpl := GetDaoRegistry(db.DB).LightwellAdvisory
	repoName := fmt.Sprintf("lightwell/java/list-osv-%d", time.Now().UnixNano())
	repoConfigUUID := s.createCommittedLightwellRepoConfig(repoName, "lightwell-network")

	published := time.Date(2026, 9, 17, 18, 39, 42, 0, time.UTC)
	modified := published
	err := daoImpl.SyncForRepository(context.Background(), repoConfigUUID, repoName, []LightwellAdvisoryInput{
		{
			AdvisoryID:     "x_RHLW-CVE-2015-6748-1.7.2",
			Severity:       "6.1",
			Details:        "Cross-site scripting (XSS) vulnerability in jsoup before 1.8.3.",
			PackageName:    "org.jsoup:jsoup",
			PackageVersion: "1.7.2",
			FixedVersions:  []string{"1.7.2.rhlw-00001"},
			Checksum:       "abc123",
			Published:      &published,
			Modified:       &modified,
			Aliases:        []string{"GHSA-48rh-qgjr-xfj6", "CVE-2015-6748"},
			SchemaVersion:  "1.6.8",
			Source:         "pnc-build",
			Summary:        "Improper Neutralization of Input During Web Page Generation in Jsoup",
		},
	})
	s.Require().NoError(err)

	data, total, err := daoImpl.ListAdvisories(context.Background(), ListLightwellAdvisoriesOptions{
		RepoName:         &repoName,
		EntitledFeatures: []string{"lightwell-network"},
		Limit:            100,
		Offset:           0,
	})
	s.Require().NoError(err)
	s.Equal(int64(1), total)
	s.Require().Len(data, 1)
	got := data[0]
	s.Equal("CVE-2015-6748", got.AdvisoryName)
	s.Equal(float32(6.1), got.SeverityScore)
	s.Equal("1.7.2", got.PackageVersion)
	s.Equal("pnc-build", got.Source)
	s.Equal("1.6.8", got.SchemaVersion)
	s.Equal("Improper Neutralization of Input During Web Page Generation in Jsoup", got.Summary)
	s.Equal([]string{"GHSA-48rh-qgjr-xfj6", "CVE-2015-6748"}, got.Aliases)
	s.Require().NotNil(got.Published)
	s.True(published.Equal(*got.Published))
	s.False(got.CreatedAt.IsZero())
	s.False(got.UpdatedAt.IsZero())
}

func (s *LightwellAdvisorySuite) TestListAdvisoriesFiltersNamePackageVersionAndCveID() {
	s.Require().NotNil(db.LightwellQueries)
	daoImpl := GetDaoRegistry(db.DB).LightwellAdvisory
	repoName := fmt.Sprintf("lightwell/java/list-filters-%d", time.Now().UnixNano())
	repoConfigUUID := s.createCommittedLightwellRepoConfig(repoName, "lightwell-network")

	err := daoImpl.SyncForRepository(context.Background(), repoConfigUUID, repoName, []LightwellAdvisoryInput{
		{
			AdvisoryID:    "x_RHLW-CVE-2015-6748-1.7.2",
			PackageName:   "org.jsoup:jsoup",
			Checksum:      "jsoup-1",
			FixedVersions: []string{"1.7.2.rhlw-00001"},
			Aliases:       []string{"GHSA-48rh-qgjr-xfj6", "CVE-2015-6748"},
		},
		{
			AdvisoryID:    "x_RHLW-LW-2026-4255-2.11.0",
			PackageName:   "com.example:other",
			Checksum:      "other-1",
			FixedVersions: []string{"2.11.0.rhlw-00000"},
			Aliases:       []string{"LW-2026-4255"},
		},
	})
	s.Require().NoError(err)

	opts := func(mutate func(*ListLightwellAdvisoriesOptions)) ListLightwellAdvisoriesOptions {
		o := ListLightwellAdvisoriesOptions{
			EntitledFeatures: []string{"lightwell-network"},
			RepoName:         &repoName,
			Limit:            100,
			Offset:           0,
		}
		mutate(&o)
		return o
	}

	alias := "GHSA-48rh"
	data, total, err := daoImpl.ListAdvisories(context.Background(), opts(func(o *ListLightwellAdvisoriesOptions) {
		o.Name = &alias
	}))
	s.Require().NoError(err)
	s.Equal(int64(1), total)
	s.Require().Len(data, 1)
	s.Equal("org.jsoup:jsoup", data[0].PackageName)

	idName := "CVE-2015-6748"
	data, total, err = daoImpl.ListAdvisories(context.Background(), opts(func(o *ListLightwellAdvisoriesOptions) {
		o.Name = &idName
	}))
	s.Require().NoError(err)
	s.Equal(int64(1), total)
	s.Equal("x_RHLW-CVE-2015-6748-1.7.2", data[0].AdvisoryID)

	ver := "1.7.2"
	data, total, err = daoImpl.ListAdvisories(context.Background(), opts(func(o *ListLightwellAdvisoriesOptions) {
		o.PackageVersion = &ver
	}))
	s.Require().NoError(err)
	s.Equal(int64(1), total)
	s.Equal("org.jsoup:jsoup", data[0].PackageName)

	data, total, err = daoImpl.ListAdvisories(context.Background(), opts(func(o *ListLightwellAdvisoriesOptions) {
		o.Name = &idName
		o.PackageVersion = &ver
	}))
	s.Require().NoError(err)
	s.Equal(int64(1), total)
	s.Equal("org.jsoup:jsoup", data[0].PackageName)

	wrongVer := "2.11.0"
	data, total, err = daoImpl.ListAdvisories(context.Background(), opts(func(o *ListLightwellAdvisoriesOptions) {
		o.Name = &idName
		o.PackageVersion = &wrongVer
	}))
	s.Require().NoError(err)
	s.Equal(int64(0), total)
	s.Empty(data)

	cveID := "CVE-2015-6748"
	data, total, err = daoImpl.ListAdvisories(context.Background(), opts(func(o *ListLightwellAdvisoriesOptions) {
		o.CveID = &cveID
	}))
	s.Require().NoError(err)
	s.Equal(int64(0), total)
	s.Empty(data)

	exactID := "x_RHLW-CVE-2015-6748-1.7.2"
	data, total, err = daoImpl.ListAdvisories(context.Background(), opts(func(o *ListLightwellAdvisoriesOptions) {
		o.CveID = &exactID
	}))
	s.Require().NoError(err)
	s.Equal(int64(1), total)
	s.Equal("org.jsoup:jsoup", data[0].PackageName)
}

func (s *LightwellAdvisorySuite) TestListAdvisoriesLatestRelease() {
	s.Require().NotNil(db.LightwellQueries)
	daoImpl := GetDaoRegistry(db.DB).LightwellAdvisory
	repoName := fmt.Sprintf("lightwell/java/list-latest-%d", time.Now().UnixNano())
	repoConfigUUID := s.createCommittedLightwellRepoConfig(repoName, "lightwell-network")

	base := []LightwellAdvisoryInput{
		{
			AdvisoryID:     "x_RHLW-CVE-2015-0001-1.2.3",
			PackageName:    "org.jsoup:jsoup",
			PackageVersion: "1.2.3",
			FixedVersions:  []string{"1.2.3.rhlw.00003"},
			Checksum:       "jsoup-rhlw-1",
			Severity:       "9.8",
		},
		{
			AdvisoryID:     "x_RHLW-CVE-2015-0002-1.2.3",
			PackageName:    "org.jsoup:jsoup",
			PackageVersion: "1.2.3",
			FixedVersions:  []string{"1.2.3.rhlw.00003.n00001"},
			Checksum:       "jsoup-rhlw-2",
			Severity:       "5.0",
		},
		{
			AdvisoryID:     "x_RHLW-CVE-2015-0003-1.2.3",
			PackageName:    "org.jsoup:jsoup",
			PackageVersion: "1.2.3",
			FixedVersions:  []string{"1.2.3.rhlw.00003.n00001.hf00001"},
			Checksum:       "jsoup-rhlw-3",
			Severity:       "4.0",
		},
		{
			AdvisoryID:     "x_RHLW-CVE-2015-0006-2.0.0",
			PackageName:    "org.jsoup:jsoup",
			PackageVersion: "2.0.0",
			FixedVersions:  []string{"2.0.0.rhlw.00009"},
			Checksum:       "jsoup-rhlw-6",
			Severity:       "10.0",
		},
		{
			AdvisoryID:     "x_RHLW-CVE-2015-0007-1.2.3",
			PackageName:    "org.jsoup:jsoup",
			PackageVersion: "1.2.3",
			FixedVersions:  []string{"1.2.3"},
			Checksum:       "jsoup-upstream",
			Severity:       "9.9",
		},
	}
	s.Require().NoError(daoImpl.SyncForRepository(context.Background(), repoConfigUUID, repoName, base))

	pkg := "jsoup"
	ver := "1.2.3"
	opts := ListLightwellAdvisoriesOptions{
		RepoName:         &repoName,
		PackageName:      &pkg,
		PackageVersion:   &ver,
		EntitledFeatures: []string{"lightwell-network"},
		Limit:            100,
		Offset:           0,
		LatestRelease:    true,
	}

	data, total, err := daoImpl.ListAdvisories(context.Background(), opts)
	s.Require().NoError(err)
	s.Equal(int64(1), total)
	s.Require().Len(data, 1)
	s.Equal("x_RHLW-CVE-2015-0003-1.2.3", data[0].AdvisoryID)

	withNextBaseline := append(base, []LightwellAdvisoryInput{
		{
			AdvisoryID:     "x_RHLW-CVE-2015-0004-1.2.3",
			PackageName:    "org.jsoup:jsoup",
			PackageVersion: "1.2.3",
			FixedVersions:  []string{"1.2.3.rhlw.00004"},
			Checksum:       "jsoup-rhlw-4",
			Severity:       "7.0",
		},
		{
			AdvisoryID:     "x_RHLW-CVE-2015-0005-1.2.3",
			PackageName:    "org.jsoup:jsoup",
			PackageVersion: "1.2.3",
			FixedVersions:  []string{"1.2.3.rhlw.00004"},
			Checksum:       "jsoup-rhlw-5",
			Severity:       "6.0",
		},
	}...)
	s.Require().NoError(daoImpl.SyncForRepository(context.Background(), repoConfigUUID, repoName, withNextBaseline))

	opts.LatestRelease = false
	data, total, err = daoImpl.ListAdvisories(context.Background(), opts)
	s.Require().NoError(err)
	s.Equal(int64(6), total)
	s.Len(data, 6)

	opts.LatestRelease = true
	data, total, err = daoImpl.ListAdvisories(context.Background(), opts)
	s.Require().NoError(err)
	s.Equal(int64(2), total)
	s.Require().Len(data, 2)
	ids := []string{data[0].AdvisoryID, data[1].AdvisoryID}
	s.ElementsMatch([]string{"x_RHLW-CVE-2015-0004-1.2.3", "x_RHLW-CVE-2015-0005-1.2.3"}, ids)
}

func (s *LightwellAdvisorySuite) TestListUnnotifiedAdvisoriesMultipleFixedVersions() {
	dao := GetLightwellAdvisoryDao(s.tx)
	repoConfigUUID := s.createLightwellRepoConfig("lightwell/notif/multi-versions")

	advisories := []LightwellAdvisoryInput{
		{AdvisoryID: "CVE-2026-0001", PackageName: "com.example:lib-a", Severity: "9.8", FixedVersions: []string{"1.0.1", "2.0.0"}, Checksum: "aaa"},
	}
	err := dao.SyncForRepository(context.Background(), repoConfigUUID, "lightwell/notif/multi-versions", advisories)
	s.Require().NoError(err)

	unnotified, err := dao.ListUnnotifiedAdvisories(context.Background(), repoConfigUUID, "org-1")
	s.NoError(err)
	s.Require().Len(unnotified, 1)
	s.Equal([]string{"1.0.1", "2.0.0"}, unnotified[0].FixedVersions)
}
