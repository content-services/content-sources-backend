package dao

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type LightwellPackageSuite struct {
	*DaoSuite
}

func TestLightwellPackageSuite(t *testing.T) {
	m := DaoSuite{}
	r := LightwellPackageSuite{
		DaoSuite: &m,
	}
	suite.Run(t, &r)
}

func (s *LightwellPackageSuite) dao() (context.Context, LightwellPackageDao) {
	s.Require().NotNil(db.LightwellQueries)
	return context.Background(), GetDaoRegistry(db.DB).LightwellPackage
}

func (s *LightwellPackageSuite) seedLightwellRepoConfig(contentType string, securityLevel string, name string) string {
	repoUUID := uuid.New().String()
	repoConfigUUID := uuid.New().String()

	err := s.tx.Exec(`
		INSERT INTO repositories (uuid, created_at, updated_at, origin, content_type, security_level, published_distribution_base_path)
		VALUES (?, NOW(), NOW(), ?, ?, ?, ?)`,
		repoUUID, config.OriginLightwell, contentType, securityLevel, "/path/to/"+name,
	).Error
	s.Require().NoError(err)

	err = s.tx.Exec(`
		INSERT INTO repository_configurations (uuid, created_at, updated_at, name, org_id, repository_uuid, feature_name, arch, versions, snapshot, label)
		VALUES (?, NOW(), NOW(), ?, ?, ?, ?, 'any', '{any}', false, ?)`,
		repoConfigUUID, name, config.LightwellOrg, repoUUID, "lightwell-"+contentType, name,
	).Error
	s.Require().NoError(err)

	s.T().Cleanup(func() {
		_ = s.tx.Exec(`DELETE FROM repository_configurations WHERE uuid = ?`, repoConfigUUID).Error
		_ = s.tx.Exec(`DELETE FROM repositories WHERE uuid = ?`, repoUUID).Error
	})

	return repoConfigUUID
}

func (s *LightwellPackageSuite) createLightwellRepoWithFeature(ctx context.Context, name string, featureName string, securityLevel string) (string, string) {
	repoUUID := uuid.New().String()
	repoConfigUUID := uuid.New().String()

	err := db.DB.WithContext(ctx).Exec(`
		INSERT INTO repositories (uuid, created_at, updated_at, origin, content_type, security_level)
		VALUES (?, NOW(), NOW(), ?, ?, ?)`,
		repoUUID, config.OriginLightwell, config.ContentTypeMaven, securityLevel,
	).Error
	s.Require().NoError(err)

	err = db.DB.WithContext(ctx).Exec(`
		INSERT INTO repository_configurations (uuid, created_at, updated_at, name, org_id, repository_uuid, feature_name, arch, versions, snapshot, label)
		VALUES (?, NOW(), NOW(), ?, ?, ?, ?, 'any', '{any}', false, ?)`,
		repoConfigUUID, name, config.LightwellOrg, repoUUID, featureName, name,
	).Error
	s.Require().NoError(err)

	s.T().Cleanup(func() {
		_ = db.DB.Exec(`DELETE FROM repository_configurations WHERE uuid = ?`, repoConfigUUID).Error
		_ = db.DB.Exec(`DELETE FROM repositories WHERE uuid = ?`, repoUUID).Error
	})

	return repoConfigUUID, repoUUID
}

func (s *LightwellPackageSuite) createPackageWithVersions(ctx context.Context, repoConfigUUID string, name string, group string, versions []string, releases []string, purls []string) string {
	pkgUUID := uuid.New().String()

	err := db.DB.WithContext(ctx).Exec(`
		INSERT INTO lightwell_packages (uuid, created_at, updated_at, repository_configuration_uuid, name, package_group)
		VALUES (?, NOW(), NOW(), ?, ?, ?)`,
		pkgUUID, repoConfigUUID, name, group,
	).Error
	s.Require().NoError(err)

	for i, version := range versions {
		release := releases[i]
		purl := purls[i]
		versionUUID := uuid.New().String()
		err = db.DB.WithContext(ctx).Exec(`
			INSERT INTO lightwell_package_versions (uuid, created_at, updated_at, lightwell_package_uuid, repository_configuration_uuid, version, release, published_at, purl)
			VALUES (?, NOW(), NOW(), ?, ?, ?, ?, ?, ?)`,
			versionUUID, pkgUUID, repoConfigUUID, version, release, "2026-09-25T00:00:00Z", purl,
		).Error
		s.Require().NoError(err)

		s.T().Cleanup(func() {
			_ = db.DB.Exec(`DELETE FROM lightwell_package_versions WHERE uuid = ?`, versionUUID).Error
		})
	}

	s.T().Cleanup(func() {
		_ = db.DB.Exec(`DELETE FROM lightwell_packages WHERE uuid = ?`, pkgUUID).Error
	})

	return pkgUUID
}

func (s *LightwellPackageSuite) createPackageWithoutVersions(ctx context.Context, repoConfigUUID string, name string, group string) string {
	pkgUUID := uuid.New().String()

	err := db.DB.WithContext(ctx).Exec(`
		INSERT INTO lightwell_packages (uuid, created_at, updated_at, repository_configuration_uuid, name, package_group)
		VALUES (?, NOW(), NOW(), ?, ?, ?)`,
		pkgUUID, repoConfigUUID, name, group,
	).Error
	s.Require().NoError(err)

	s.T().Cleanup(func() {
		_ = db.DB.Exec(`DELETE FROM lightwell_packages WHERE uuid = ?`, pkgUUID).Error
	})

	return pkgUUID
}

func (s *LightwellPackageSuite) createAdvisory(ctx context.Context, repoConfigUUID string, advisoryID string, packageName string, fixedVersions []string) {
	advisoryUUID := uuid.New().String()

	err := db.DB.WithContext(ctx).Exec(`
		INSERT INTO lightwell_advisories (uuid, created_at, updated_at, repository_configuration_uuid, advisory_id, package_name, fixed_versions, repo_name, severity, severity_score, checksum)
		VALUES (?, NOW(), NOW(), ?, ?, ?, ?, ?, ?, ?, ?)`,
		advisoryUUID, repoConfigUUID, advisoryID, packageName, pq.Array(fixedVersions), "test-repo", "7.5", 7.5, "test-checksum",
	).Error
	s.Require().NoError(err)

	s.T().Cleanup(func() {
		_ = db.DB.Exec(`DELETE FROM lightwell_advisories WHERE uuid = ?`, advisoryUUID).Error
	})
}

func ptr(s string) *string {
	return &s
}

func (s *LightwellPackageSuite) TestListPackagesAggregatesVersions() {
	ctx, dao := s.dao()
	testID := fmt.Sprintf("agg-%d", time.Now().UnixNano())
	repoConfigUUID, _ := s.createLightwellRepoWithFeature(ctx, testID, "lightwell-maven", "validated")

	s.createPackageWithVersions(
		ctx,
		repoConfigUUID,
		"com.example:commons-lib",
		"com.example",
		[]string{"1.0", "2.0"},
		[]string{"", ""},
		[]string{"pkg:maven/com.example/commons-lib@1.0", "pkg:maven/com.example/commons-lib@2.0"},
	)

	rows, total, err := dao.ListPackages(ctx, ListLightwellPackagesOptions{
		Repository:       ptr(testID),
		EntitledFeatures: []string{"lightwell-maven"},
		Limit:            100,
		Offset:           0,
	})

	s.NoError(err)
	assert.Equal(s.T(), int64(1), total)
	s.Require().Len(rows, 1)
	assert.ElementsMatch(s.T(), []string{"1.0", "2.0"}, rows[0].Versions)
	assert.Equal(s.T(), "com.example:commons-lib", rows[0].Name)
	assert.Equal(s.T(), "com.example", rows[0].Group)
	assert.Equal(s.T(), "maven", rows[0].Ecosystem)
}

func (s *LightwellPackageSuite) TestListPackagesZeroVersions() {
	ctx, dao := s.dao()
	testID := fmt.Sprintf("zero-ver-%d", time.Now().UnixNano())
	repoConfigUUID, _ := s.createLightwellRepoWithFeature(ctx, testID, "lightwell-maven", "validated")

	s.createPackageWithoutVersions(ctx, repoConfigUUID, "com.example:no-versions", "com.example")

	rows, total, err := dao.ListPackages(ctx, ListLightwellPackagesOptions{
		Repository:       ptr(testID),
		EntitledFeatures: []string{"lightwell-maven"},
		Limit:            100,
		Offset:           0,
	})

	s.NoError(err)
	assert.Equal(s.T(), int64(1), total)
	s.Require().Len(rows, 1)
	assert.Equal(s.T(), "com.example:no-versions", rows[0].Name)
	assert.Empty(s.T(), rows[0].Versions, "Versions should be empty slice, not ['']")
	assert.Empty(s.T(), rows[0].Releases, "Releases should be empty slice, not ['']")
	assert.Empty(s.T(), rows[0].PublishedAts, "PublishedAts should be empty slice, not ['']")
	assert.Len(s.T(), rows[0].Versions, 0, "Versions length should be 0")
}

func (s *LightwellPackageSuite) TestListPackagesNameFilterCaseInsensitive() {
	ctx, dao := s.dao()
	testID := fmt.Sprintf("name-%d", time.Now().UnixNano())
	repoConfigUUID, _ := s.createLightwellRepoWithFeature(ctx, testID, "lightwell-maven", "validated")

	s.createPackageWithVersions(
		ctx,
		repoConfigUUID,
		"com.example:commons-lib",
		"com.example",
		[]string{"1.0"},
		[]string{""},
		[]string{"pkg:maven/com.example/commons-lib@1.0"},
	)

	rows, _, err := dao.ListPackages(ctx, ListLightwellPackagesOptions{
		Repository:       ptr(testID),
		Name:             ptr("COMM"),
		EntitledFeatures: []string{"lightwell-maven"},
		Limit:            100,
		Offset:           0,
	})

	s.NoError(err)
	assert.Len(s.T(), rows, 1)
	assert.Equal(s.T(), "com.example:commons-lib", rows[0].Name)
}

func (s *LightwellPackageSuite) TestListPackagesSecurityLevelFilterCaseInsensitive() {
	ctx, dao := s.dao()
	testID := fmt.Sprintf("sec-%d", time.Now().UnixNano())
	repoConfigUUID, _ := s.createLightwellRepoWithFeature(ctx, testID, "lightwell-maven", "validated")

	s.createPackageWithVersions(
		ctx,
		repoConfigUUID,
		"com.example:commons-lib",
		"com.example",
		[]string{"1.0"},
		[]string{""},
		[]string{"pkg:maven/com.example/commons-lib@1.0"},
	)

	rows, _, err := dao.ListPackages(ctx, ListLightwellPackagesOptions{
		Repository:       ptr(testID),
		SecurityLevel:    ptr("VALIDATED"),
		EntitledFeatures: []string{"lightwell-maven"},
		Limit:            100,
		Offset:           0,
	})

	s.NoError(err)
	assert.Len(s.T(), rows, 1)
	assert.Equal(s.T(), "com.example:commons-lib", rows[0].Name)
}

func (s *LightwellPackageSuite) TestListPackagesEntitlementExcludes() {
	ctx, dao := s.dao()
	testID := fmt.Sprintf("ent-%d", time.Now().UnixNano())
	repoConfigUUID, _ := s.createLightwellRepoWithFeature(ctx, testID, "lightwell-maven", "validated")

	s.createPackageWithVersions(
		ctx,
		repoConfigUUID,
		"com.example:commons-lib",
		"com.example",
		[]string{"1.0"},
		[]string{""},
		[]string{"pkg:maven/com.example/commons-lib@1.0"},
	)

	rows, total, err := dao.ListPackages(ctx, ListLightwellPackagesOptions{
		Repository:       ptr(testID),
		EntitledFeatures: []string{"some-other-feature"},
		Limit:            100,
		Offset:           0,
	})

	s.NoError(err)
	assert.Equal(s.T(), int64(0), total)
	assert.Empty(s.T(), rows)
}

// TestListPackagesBlankFeatureNameHidden verifies the entitlement gate is
// fail-closed: a Lightwell repo with a blank feature_name is entitled to no
// org and must NOT appear in results, even for an entitled caller. (Matches
// advisories.sql; the earlier fail-open branches were removed for security.)
func (s *LightwellPackageSuite) TestListPackagesBlankFeatureNameHidden() {
	ctx, dao := s.dao()
	testID := fmt.Sprintf("blank-%d", time.Now().UnixNano())
	repoConfigUUID1, _ := s.createLightwellRepoWithFeature(ctx, testID, "", "validated")

	s.createPackageWithVersions(
		ctx,
		repoConfigUUID1,
		"com.example:public-lib",
		"com.example",
		[]string{"1.0"},
		[]string{""},
		[]string{"pkg:maven/com.example/public-lib@1.0"},
	)

	rows, total, err := dao.ListPackages(ctx, ListLightwellPackagesOptions{
		Repository:       ptr(testID),
		EntitledFeatures: []string{"lightwell-maven"},
		Limit:            100,
		Offset:           0,
	})

	s.NoError(err)
	assert.Equal(s.T(), int64(0), total)
	assert.Empty(s.T(), rows)
}

func (s *LightwellPackageSuite) TestListPackageVersionsReturnsPurlAndCount() {
	ctx, dao := s.dao()
	testID := fmt.Sprintf("purl-%d", time.Now().UnixNano())
	repoConfigUUID, _ := s.createLightwellRepoWithFeature(ctx, testID, "lightwell-maven", "validated")

	s.createPackageWithVersions(
		ctx,
		repoConfigUUID,
		"com.example:commons-lib",
		"com.example",
		[]string{"1.0", "2.0"},
		[]string{"", ""},
		[]string{"pkg:maven/com.example/commons-lib@1.0", "pkg:maven/com.example/commons-lib@2.0"},
	)

	vrows, vtotal, err := dao.ListPackageVersions(ctx, ListLightwellPackageVersionsOptions{
		Repository:       ptr(testID),
		EntitledFeatures: []string{"lightwell-maven"},
		Limit:            100,
		Offset:           0,
	})

	s.NoError(err)
	assert.Equal(s.T(), int64(2), vtotal)
	s.Require().Len(vrows, 2)
	assert.NotEmpty(s.T(), vrows[0].Purl)
	assert.Contains(s.T(), vrows[0].Purl, "pkg:maven/com.example/commons-lib")
}

func (s *LightwellPackageSuite) TestListPackageVersionsResolvesCveId() {
	ctx, dao := s.dao()
	testID := fmt.Sprintf("cve-res-%d", time.Now().UnixNano())
	repoConfigUUID, _ := s.createLightwellRepoWithFeature(ctx, testID, "lightwell-maven", "validated")

	s.createPackageWithVersions(
		ctx,
		repoConfigUUID,
		"com.example:commons-lib",
		"com.example",
		[]string{"1.0", "2.0"},
		[]string{"", ""},
		[]string{"pkg:maven/com.example/commons-lib@1.0", "pkg:maven/com.example/commons-lib@2.0"},
	)

	s.createAdvisory(ctx, repoConfigUUID, "CVE-1", "com.example:commons-lib", []string{"2.0"})

	vrows, _, err := dao.ListPackageVersions(ctx, ListLightwellPackageVersionsOptions{
		Repository:       ptr(testID),
		ResolvesCveID:    ptr("CVE-1"),
		EntitledFeatures: []string{"lightwell-maven"},
		Limit:            100,
		Offset:           0,
	})

	s.NoError(err)
	s.Require().Len(vrows, 1)
	assert.Equal(s.T(), "2.0", vrows[0].Version)
}

func (s *LightwellPackageSuite) TestListPackageVersionsVulnerableToCveId() {
	ctx, dao := s.dao()
	testID := fmt.Sprintf("cve-vuln-%d", time.Now().UnixNano())
	repoConfigUUID, _ := s.createLightwellRepoWithFeature(ctx, testID, "lightwell-maven", "validated")

	s.createPackageWithVersions(
		ctx,
		repoConfigUUID,
		"com.example:commons-lib",
		"com.example",
		[]string{"1.0", "2.0"},
		[]string{"", ""},
		[]string{"pkg:maven/com.example/commons-lib@1.0", "pkg:maven/com.example/commons-lib@2.0"},
	)

	s.createAdvisory(ctx, repoConfigUUID, "CVE-1", "com.example:commons-lib", []string{"2.0"})

	vrows, _, err := dao.ListPackageVersions(ctx, ListLightwellPackageVersionsOptions{
		Repository:        ptr(testID),
		VulnerableToCveID: ptr("CVE-1"),
		EntitledFeatures:  []string{"lightwell-maven"},
		Limit:             100,
		Offset:            0,
	})

	s.NoError(err)
	s.Require().Len(vrows, 1)
	assert.Equal(s.T(), "1.0", vrows[0].Version)
}

func (s *LightwellPackageSuite) TestSyncPackagesForRepositoryUpsertAndDeleteDiff() {
	t := s.T()
	ctx := context.Background()
	dao := GetLightwellPackageDao(s.tx)
	rcUUID := s.seedLightwellRepoConfig("maven", "validated", "lightwell-maven")

	// initial import: two packages
	err := dao.SyncPackagesForRepository(ctx, rcUUID, []LightwellPackageInput{
		{Name: "commons", Group: "org.apache", Versions: []LightwellPackageVersionInput{
			{Version: "1.0", Release: "", PublishedAt: "2020-01-01T00:00:00Z", Purl: "pkg:maven/org.apache/commons@1.0"},
			{Version: "2.0", Purl: "pkg:maven/org.apache/commons@2.0"},
		}},
		{Name: "logging", Group: "org.apache", Versions: []LightwellPackageVersionInput{
			{Version: "1.5", Purl: "pkg:maven/org.apache/logging@1.5"},
		}},
	})
	require.NoError(t, err)

	var pkgCount, verCount int64
	s.tx.Model(&models.LightwellPackage{}).Where("repository_configuration_uuid = ?", rcUUID).Count(&pkgCount)
	s.tx.Model(&models.LightwellPackageVersion{}).Where("repository_configuration_uuid = ?", rcUUID).Count(&verCount)
	assert.Equal(t, int64(2), pkgCount)
	assert.Equal(t, int64(3), verCount)

	// capture a stable uuid to prove upsert keeps it
	var before models.LightwellPackage
	s.tx.Where("repository_configuration_uuid = ? AND name = ?", rcUUID, "commons").First(&before)

	// second import: "logging" removed, "commons" drops 1.0, 2.0 purl updated
	err = dao.SyncPackagesForRepository(ctx, rcUUID, []LightwellPackageInput{
		{Name: "commons", Group: "org.apache", Versions: []LightwellPackageVersionInput{
			{Version: "2.0", Purl: "pkg:maven/org.apache/commons@2.0-updated"},
		}},
	})
	require.NoError(t, err)

	s.tx.Model(&models.LightwellPackage{}).Where("repository_configuration_uuid = ?", rcUUID).Count(&pkgCount)
	s.tx.Model(&models.LightwellPackageVersion{}).Where("repository_configuration_uuid = ?", rcUUID).Count(&verCount)
	assert.Equal(t, int64(1), pkgCount) // logging deleted
	assert.Equal(t, int64(1), verCount) // only commons 2.0 remains

	var after models.LightwellPackage
	s.tx.Where("repository_configuration_uuid = ? AND name = ?", rcUUID, "commons").First(&after)
	assert.Equal(t, before.UUID, after.UUID) // upsert kept the uuid

	var ver models.LightwellPackageVersion
	s.tx.Where("lightwell_package_uuid = ?", after.UUID).First(&ver)
	assert.Equal(t, "pkg:maven/org.apache/commons@2.0-updated", ver.Purl) // updated in place
}

func (s *LightwellPackageSuite) TestSyncPackagesEmptyDeletesAll() {
	t := s.T()
	ctx := context.Background()
	dao := GetLightwellPackageDao(s.tx)
	rcUUID := s.seedLightwellRepoConfig("python", "validated", "lightwell-python")
	require.NoError(t, dao.SyncPackagesForRepository(ctx, rcUUID, []LightwellPackageInput{
		{Name: "requests", Group: "", Versions: []LightwellPackageVersionInput{{Version: "2.0", Purl: "pkg:pypi/requests@2.0"}}},
	}))
	require.NoError(t, dao.SyncPackagesForRepository(ctx, rcUUID, []LightwellPackageInput{}))
	var pkgCount int64
	s.tx.Model(&models.LightwellPackage{}).Where("repository_configuration_uuid = ?", rcUUID).Count(&pkgCount)
	assert.Equal(t, int64(0), pkgCount)
}
