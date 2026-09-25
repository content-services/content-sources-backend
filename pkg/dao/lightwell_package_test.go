package dao

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type LightwellPackageSuite struct {
	suite.Suite
}

func TestLightwellPackageSuite(t *testing.T) {
	suite.Run(t, new(LightwellPackageSuite))
}

func (s *LightwellPackageSuite) dao() (context.Context, LightwellPackageDao) {
	s.Require().NotNil(db.LightwellQueries)
	return context.Background(), GetDaoRegistry(db.DB).LightwellPackage
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

func (s *LightwellPackageSuite) TestListPackagesBlankFeatureNameVisible() {
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
	assert.Equal(s.T(), int64(1), total)
	s.Require().Len(rows, 1)
	assert.Equal(s.T(), "com.example:public-lib", rows[0].Name)
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
