package dao

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/db"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
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

func (s *LightwellPackageSuite) createLightwellRepoWithFeature(ctx context.Context, name string, featureName string, securityLevel string, contentType string) (string, string) {
	repoUUID := uuid.New().String()
	repoConfigUUID := uuid.New().String()

	err := db.DB.WithContext(ctx).Exec(`
		INSERT INTO repositories (uuid, created_at, updated_at, origin, content_type, security_level)
		VALUES (?, NOW(), NOW(), ?, ?, ?)`,
		repoUUID, config.OriginLightwell, contentType, securityLevel,
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

// createLightwellRepoWithOrg seeds a Lightwell repo under a specific org, used to
// distinguish production (LightwellOrg) from demo (LightwellDemoOrg) repos.
func (s *LightwellPackageSuite) createLightwellRepoWithOrg(ctx context.Context, name string, featureName string, securityLevel string, orgID string) (string, string) {
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
		repoConfigUUID, name, orgID, repoUUID, featureName, name,
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

func (s *LightwellPackageSuite) TestInternalOnlyListLensCatalog() {
	ctx, dao := s.dao()
	suffix := time.Now().UnixNano()

	javaUUID, _ := s.createLightwellRepoWithFeature(
		ctx,
		fmt.Sprintf("lens-java-%d", suffix),
		"lightwell-maven",
		"validated",
		config.ContentTypeMaven,
	)
	pythonUUID, _ := s.createLightwellRepoWithFeature(
		ctx,
		fmt.Sprintf("lens-python-%d", suffix),
		"lightwell-python",
		"remediated",
		config.ContentTypePython,
	)
	otherUUID, _ := s.createLightwellRepoWithFeature(
		ctx,
		fmt.Sprintf("lens-other-%d", suffix),
		"lightwell-maven",
		"predisclosure",
		config.ContentTypeMaven,
	)

	s.createPackageWithVersions(
		ctx, javaUUID, "spring-core", "org.springframework",
		[]string{"6.1.0"}, []string{""}, []string{"pkg:maven/org.springframework/spring-core@6.1.0"},
	)
	s.createPackageWithVersions(
		ctx, pythonUUID, "flask", "",
		[]string{"3.0.3"}, []string{""}, []string{"pkg:pypi/flask@3.0.3"},
	)
	s.createPackageWithVersions(
		ctx, otherUUID, "ignored", "com.example",
		[]string{"1.0.0"}, []string{""}, []string{"pkg:maven/com.example/ignored@1.0.0"},
	)

	rows, err := dao.InternalOnlyListLensCatalog(ctx)
	s.NoError(err)

	found := map[string]LensCatalogPackageRow{}
	for _, row := range rows {
		found[row.Ecosystem+":"+row.Group+":"+row.Name+":"+row.Version] = row
	}
	_, hasJava := found["maven:org.springframework:spring-core:6.1.0"]
	_, hasPython := found["python::flask:3.0.3"]
	_, hasOther := found["maven:com.example:ignored:1.0.0"]
	assert.True(s.T(), hasJava)
	assert.True(s.T(), hasPython)
	assert.False(s.T(), hasOther)
}

func (s *LightwellPackageSuite) TestListPackagesAggregatesVersions() {
	ctx, dao := s.dao()
	testID := fmt.Sprintf("agg-%d", time.Now().UnixNano())
	repoConfigUUID, _ := s.createLightwellRepoWithFeature(ctx, testID, "lightwell-maven", "validated", config.ContentTypeMaven)

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

// TestListPackagesDemoFilter verifies demo repos (LightwellDemoOrg) are excluded
// by default and returned exclusively when Demo is true, so the two sets never
// overlap. Package names embed a unique suffix so the Name filter isolates this
// test's rows on the shared database.
func (s *LightwellPackageSuite) TestListPackagesDemoFilter() {
	ctx, dao := s.dao()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	pkgPrefix := "com.example:demotest-" + suffix

	prodRepo, _ := s.createLightwellRepoWithOrg(ctx, "prod-"+suffix, "lightwell-maven", "validated", config.LightwellOrg)
	demoRepo, _ := s.createLightwellRepoWithOrg(ctx, "demo-"+suffix, "lightwell-maven", "validated", config.LightwellDemoOrg)

	prodPkg := pkgPrefix + "-prod"
	demoPkg := pkgPrefix + "-demo"
	s.createPackageWithVersions(ctx, prodRepo, prodPkg, "com.example",
		[]string{"1.0"}, []string{""}, []string{"pkg:maven/com.example/prod-lib@1.0"})
	s.createPackageWithVersions(ctx, demoRepo, demoPkg, "com.example",
		[]string{"9.0"}, []string{""}, []string{"pkg:maven/com.example/demo-lib@9.0"})

	// Default (Demo=false): only the production repo's package.
	rows, total, err := dao.ListPackages(ctx, ListLightwellPackagesOptions{
		Name:             ptr(pkgPrefix),
		EntitledFeatures: []string{"lightwell-maven"},
		Limit:            100,
		Offset:           0,
	})
	s.NoError(err)
	assert.Equal(s.T(), int64(1), total, "default should return only the production package")
	s.Require().Len(rows, 1)
	assert.Equal(s.T(), prodPkg, rows[0].Name)
	assert.Equal(s.T(), prodRepo, rows[0].RepositoryConfigurationUUID)

	// Demo=true: only the demo repo's package.
	demoRows, demoTotal, err := dao.ListPackages(ctx, ListLightwellPackagesOptions{
		Name:             ptr(pkgPrefix),
		Demo:             true,
		EntitledFeatures: []string{"lightwell-maven"},
		Limit:            100,
		Offset:           0,
	})
	s.NoError(err)
	assert.Equal(s.T(), int64(1), demoTotal, "demo=true should return only the demo package")
	s.Require().Len(demoRows, 1)
	assert.Equal(s.T(), demoPkg, demoRows[0].Name)
	assert.Equal(s.T(), demoRepo, demoRows[0].RepositoryConfigurationUUID)
}

func (s *LightwellPackageSuite) TestListPackagesZeroVersions() {
	ctx, dao := s.dao()
	testID := fmt.Sprintf("zero-ver-%d", time.Now().UnixNano())
	repoConfigUUID, _ := s.createLightwellRepoWithFeature(ctx, testID, "lightwell-maven", "validated", config.ContentTypeMaven)

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
	assert.Empty(s.T(), rows[0].UpstreamVersions, "UpstreamVersions should be empty slice, not ['']")
	assert.Len(s.T(), rows[0].Versions, 0, "Versions length should be 0")
}

func (s *LightwellPackageSuite) TestListPackagesNameFilterCaseInsensitive() {
	ctx, dao := s.dao()
	testID := fmt.Sprintf("name-%d", time.Now().UnixNano())
	repoConfigUUID, _ := s.createLightwellRepoWithFeature(ctx, testID, "lightwell-maven", "validated", config.ContentTypeMaven)

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
	assert.Equal(s.T(), []string{""}, rows[0].UpstreamVersions)
}

func (s *LightwellPackageSuite) TestListPackagesSearchMatchesGroup() {
	ctx, dao := s.dao()
	testID := fmt.Sprintf("search-%d", time.Now().UnixNano())
	repoConfigUUID, _ := s.createLightwellRepoWithFeature(ctx, testID, "lightwell-maven", "validated", config.ContentTypeMaven)

	// Match package: name does not contain the search term; group does.
	s.createPackageWithVersions(ctx, repoConfigUUID, "widget-lib", "com.uniquegroup",
		[]string{"1.0"}, []string{""}, []string{"pkg:maven/com.uniquegroup/widget-lib@1.0"})
	// Non-match package: neither name nor group contains the search term. Proves the
	// search param actually filters rather than being ignored.
	s.createPackageWithVersions(ctx, repoConfigUUID, "other-lib", "com.other",
		[]string{"1.0"}, []string{""}, []string{"pkg:maven/com.other/other-lib@1.0"})

	rows, _, err := dao.ListPackages(ctx, ListLightwellPackagesOptions{
		Repository:       ptr(testID),
		Search:           ptr("uniquegroup"),
		EntitledFeatures: []string{"lightwell-maven"},
		Limit:            100,
		Offset:           0,
	})

	s.NoError(err)
	s.Require().Len(rows, 1)
	assert.Equal(s.T(), "widget-lib", rows[0].Name)
	assert.Equal(s.T(), "com.uniquegroup", rows[0].Group)
}

func (s *LightwellPackageSuite) TestListPackagesSortByGroupNameDesc() {
	ctx, dao := s.dao()
	testID := fmt.Sprintf("sort-gn-%d", time.Now().UnixNano())
	repoConfigUUID, _ := s.createLightwellRepoWithFeature(ctx, testID, "lightwell-maven", "validated", config.ContentTypeMaven)

	s.createPackageWithVersions(ctx, repoConfigUUID, "alpha", "com.aaa",
		[]string{"1.0"}, []string{""}, []string{"pkg:maven/com.aaa/alpha@1.0"})
	s.createPackageWithVersions(ctx, repoConfigUUID, "beta", "com.zzz",
		[]string{"1.0"}, []string{""}, []string{"pkg:maven/com.zzz/beta@1.0"})

	rows, _, err := dao.ListPackages(ctx, ListLightwellPackagesOptions{
		Repository:       ptr(testID),
		SortColumn:       "group_name",
		SortDirection:    "desc",
		EntitledFeatures: []string{"lightwell-maven"},
		Limit:            100,
		Offset:           0,
	})

	s.NoError(err)
	s.Require().Len(rows, 2)
	assert.Equal(s.T(), "com.zzz", rows[0].Group, "descending group order should put com.zzz first")
	assert.Equal(s.T(), "com.aaa", rows[1].Group)
}

func (s *LightwellPackageSuite) TestListPackagesSortByLastPublication() {
	ctx, dao := s.dao()
	testID := fmt.Sprintf("sort-lp-%d", time.Now().UnixNano())
	repoConfigUUID, _ := s.createLightwellRepoWithFeature(ctx, testID, "lightwell-maven", "validated", config.ContentTypeMaven)

	// Names are chosen so alphabetical (default) order is the OPPOSITE of recency
	// order: "aaa-stale" sorts first by name but was published longest ago. This proves
	// last_publication:desc sorts by the version publish timestamp, not the default name ordering.
	stalePkg := s.createPackageWithVersions(ctx, repoConfigUUID, "aaa-stale", "com.example",
		[]string{"1.0"}, []string{""}, []string{"pkg:maven/com.example/aaa-stale@1.0"})
	recentPkg := s.createPackageWithVersions(ctx, repoConfigUUID, "zzz-recent", "com.example",
		[]string{"1.0"}, []string{""}, []string{"pkg:maven/com.example/zzz-recent@1.0"})

	// last_publication sorts on the latest version publish time. published_at is a text
	// column storing RFC3339 timestamps; set deterministic, distinct values.
	s.Require().NoError(db.DB.WithContext(ctx).Exec(
		`UPDATE lightwell_package_versions SET published_at = ? WHERE lightwell_package_uuid = ?`, "2020-01-01T00:00:00Z", stalePkg).Error)
	s.Require().NoError(db.DB.WithContext(ctx).Exec(
		`UPDATE lightwell_package_versions SET published_at = ? WHERE lightwell_package_uuid = ?`, "2025-01-01T00:00:00Z", recentPkg).Error)

	rows, _, err := dao.ListPackages(ctx, ListLightwellPackagesOptions{
		Repository:       ptr(testID),
		SortColumn:       "last_publication",
		SortDirection:    "desc",
		EntitledFeatures: []string{"lightwell-maven"},
		Limit:            100,
		Offset:           0,
	})

	s.NoError(err)
	s.Require().Len(rows, 2)
	assert.Equal(s.T(), "zzz-recent", rows[0].Name, "most recently published should be first with last_publication:desc")
	assert.Equal(s.T(), "aaa-stale", rows[1].Name)
	assert.Equal(s.T(), "2025-01-01T00:00:00Z", rows[0].LastPublication)
}

func (s *LightwellPackageSuite) TestListPackagesSecurityLevelFilterCaseInsensitive() {
	ctx, dao := s.dao()
	testID := fmt.Sprintf("sec-%d", time.Now().UnixNano())
	repoConfigUUID, _ := s.createLightwellRepoWithFeature(ctx, testID, "lightwell-maven", "validated", config.ContentTypeMaven)

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
	repoConfigUUID, _ := s.createLightwellRepoWithFeature(ctx, testID, "lightwell-maven", "validated", config.ContentTypeMaven)

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
	repoConfigUUID1, _ := s.createLightwellRepoWithFeature(ctx, testID, "", "validated", config.ContentTypeMaven)

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
	repoConfigUUID, _ := s.createLightwellRepoWithFeature(ctx, testID, "lightwell-maven", "validated", config.ContentTypeMaven)

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
	assert.Equal(s.T(), "", vrows[0].UpstreamVersion)
	assert.Equal(s.T(), "", vrows[1].UpstreamVersion)
}

// TestListPackageVersionsDemoFilter verifies the package_versions query excludes
// demo repos by default and returns them exclusively when Demo is true.
func (s *LightwellPackageSuite) TestListPackageVersionsDemoFilter() {
	ctx, dao := s.dao()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	pkgPrefix := "com.example:demover-" + suffix

	prodRepo, _ := s.createLightwellRepoWithOrg(ctx, "vprod-"+suffix, "lightwell-maven", "validated", config.LightwellOrg)
	demoRepo, _ := s.createLightwellRepoWithOrg(ctx, "vdemo-"+suffix, "lightwell-maven", "validated", config.LightwellDemoOrg)

	s.createPackageWithVersions(ctx, prodRepo, pkgPrefix+"-prod", "com.example",
		[]string{"1.0"}, []string{""}, []string{"pkg:maven/com.example/prod-lib@1.0"})
	s.createPackageWithVersions(ctx, demoRepo, pkgPrefix+"-demo", "com.example",
		[]string{"9.0"}, []string{""}, []string{"pkg:maven/com.example/demo-lib@9.0"})

	rows, total, err := dao.ListPackageVersions(ctx, ListLightwellPackageVersionsOptions{
		Name:             ptr(pkgPrefix),
		EntitledFeatures: []string{"lightwell-maven"},
		Limit:            100,
		Offset:           0,
	})
	s.NoError(err)
	assert.Equal(s.T(), int64(1), total)
	s.Require().Len(rows, 1)
	assert.Equal(s.T(), prodRepo, rows[0].RepositoryConfigurationUUID)
	assert.Equal(s.T(), "1.0", rows[0].Version)

	demoRows, demoTotal, err := dao.ListPackageVersions(ctx, ListLightwellPackageVersionsOptions{
		Name:             ptr(pkgPrefix),
		Demo:             true,
		EntitledFeatures: []string{"lightwell-maven"},
		Limit:            100,
		Offset:           0,
	})
	s.NoError(err)
	assert.Equal(s.T(), int64(1), demoTotal)
	s.Require().Len(demoRows, 1)
	assert.Equal(s.T(), demoRepo, demoRows[0].RepositoryConfigurationUUID)
	assert.Equal(s.T(), "9.0", demoRows[0].Version)
}

func (s *LightwellPackageSuite) TestListPackageVersionsResolvesCveId() {
	ctx, dao := s.dao()
	testID := fmt.Sprintf("cve-res-%d", time.Now().UnixNano())
	repoConfigUUID, _ := s.createLightwellRepoWithFeature(ctx, testID, "lightwell-maven", "validated", config.ContentTypeMaven)

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
	repoConfigUUID, _ := s.createLightwellRepoWithFeature(ctx, testID, "lightwell-maven", "validated", config.ContentTypeMaven)

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
	assert.Equal(t, "2.0", ver.UpstreamVersion)
	assert.Equal(t, "", ver.Release)
}

func (s *LightwellPackageSuite) TestSyncPackagesStoresVersionMetadata() {
	t := s.T()
	ctx := context.Background()
	dao := GetLightwellPackageDao(s.tx)
	rcUUID := s.seedLightwellRepoConfig("maven", "validated", "lightwell-maven-details")

	details := &LightwellPackageVersionDetailsInput{
		ProjectURL:  "https://example.com/commons",
		License:     "Apache-2.0",
		Summary:     "commons summary",
		Description: "commons description",
		Author:      "Apache",
	}
	err := dao.SyncPackagesForRepository(ctx, rcUUID, []LightwellPackageInput{
		{Name: "commons", Group: "org.apache", Versions: []LightwellPackageVersionInput{
			{Version: "1.2.3", Purl: "pkg:maven/org.apache/commons@1.2.3", Details: details},
			{Version: "1.2.3.rhlw-00001", Purl: "pkg:maven/org.apache/commons@1.2.3.rhlw-00001", Details: details},
			{Version: "5.3.18.lw-1", Purl: "pkg:maven/org.apache/commons@5.3.18.lw-1"},
		}},
	})
	require.NoError(t, err)

	var versions []models.LightwellPackageVersion
	require.NoError(t, s.tx.Where("repository_configuration_uuid = ?", rcUUID).Order("version").Find(&versions).Error)
	require.Len(t, versions, 3)
	byVersion := map[string]models.LightwellPackageVersion{}
	for _, ver := range versions {
		byVersion[ver.Version] = ver
	}
	assert.Equal(t, "1.2.3", byVersion["1.2.3"].UpstreamVersion)
	assert.Equal(t, "", byVersion["1.2.3"].Release)
	assert.Equal(t, "1.2.3", byVersion["1.2.3.rhlw-00001"].UpstreamVersion)
	assert.Equal(t, "rhlw-00001", byVersion["1.2.3.rhlw-00001"].Release)
	assert.Equal(t, "5.3.18.lw-1", byVersion["5.3.18.lw-1"].UpstreamVersion)
	assert.Equal(t, "", byVersion["5.3.18.lw-1"].Release)

	assert.Equal(t, "Apache", byVersion["1.2.3"].Author)
	assert.Equal(t, "commons summary", byVersion["1.2.3"].Summary)
	assert.Equal(t, "", byVersion["1.2.3"].AuthorEmail)
	assert.Equal(t, "Apache", byVersion["1.2.3.rhlw-00001"].Author)
	assert.Equal(t, "commons description", byVersion["1.2.3.rhlw-00001"].Description)
	assert.Equal(t, "", byVersion["5.3.18.lw-1"].Summary)
	upstreamUUID := byVersion["1.2.3"].UUID

	err = dao.SyncPackagesForRepository(ctx, rcUUID, []LightwellPackageInput{
		{Name: "commons", Group: "org.apache", Versions: []LightwellPackageVersionInput{
			{Version: "1.2.3", Purl: "pkg:maven/org.apache/commons@1.2.3", Details: &LightwellPackageVersionDetailsInput{
				Summary: "updated summary",
				Author:  "Apache",
			}},
		}},
	})
	require.NoError(t, err)

	var verCount int64
	s.tx.Model(&models.LightwellPackageVersion{}).Where("repository_configuration_uuid = ?", rcUUID).Count(&verCount)
	assert.Equal(t, int64(1), verCount)

	var remaining models.LightwellPackageVersion
	require.NoError(t, s.tx.Where("uuid = ?", upstreamUUID).First(&remaining).Error)
	assert.Equal(t, "1.2.3", remaining.UpstreamVersion)
	assert.Equal(t, "updated summary", remaining.Summary)
	assert.Equal(t, "Apache", remaining.Author)

	err = dao.SyncPackagesForRepository(ctx, rcUUID, []LightwellPackageInput{
		{Name: "commons", Group: "org.apache", Versions: []LightwellPackageVersionInput{
			{Version: "1.2.3", Purl: "pkg:maven/org.apache/commons@1.2.3"},
		}},
	})
	require.NoError(t, err)
	var kept models.LightwellPackageVersion
	require.NoError(t, s.tx.Where("uuid = ?", upstreamUUID).First(&kept).Error)
	assert.Equal(t, "updated summary", kept.Summary)

	err = dao.SyncPackagesForRepository(ctx, rcUUID, []LightwellPackageInput{
		{Name: "commons", Group: "org.apache", Versions: []LightwellPackageVersionInput{
			{Version: "9.0", Purl: "pkg:maven/org.apache/commons@9.0"},
		}},
	})
	require.NoError(t, err)
	var nine models.LightwellPackageVersion
	require.NoError(t, s.tx.Where("repository_configuration_uuid = ? AND version = ?", rcUUID, "9.0").First(&nine).Error)
	assert.Equal(t, "", nine.Summary)
	assert.Equal(t, "", nine.Author)
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

func (s *LightwellPackageSuite) TestGetPythonPackageDetail() {
	ctx, pkgDao := s.dao()
	repoConfigUUID, _ := s.createLightwellRepoWithFeature(
		ctx,
		fmt.Sprintf("python-detail-%s", uuid.NewString()),
		"lightwell-python",
		"remediated",
		config.ContentTypePython,
	)

	pkgUUID := uuid.NewString()
	err := db.DB.WithContext(ctx).Exec(`
		INSERT INTO lightwell_packages (uuid, created_at, updated_at, repository_configuration_uuid, name, package_group)
		VALUES (?, NOW(), NOW(), ?, ?, ?)`,
		pkgUUID, repoConfigUUID, "django", "",
	).Error
	s.Require().NoError(err)
	s.T().Cleanup(func() {
		_ = db.DB.Exec(`DELETE FROM lightwell_packages WHERE uuid = ?`, pkgUUID).Error
	})

	rows := []struct {
		version, release, upstream, published, summary, description, license, author, authorEmail, projectURL string
	}{
		{
			version: "4.25.7+rhlw.1", release: "rhlw.1", upstream: "4.25.7", published: "2024-01-01T00:00:00Z",
			summary: "older summary", license: "BSD-3-Clause", author: "Django Software Foundation",
			authorEmail: "foundation@djangoproject.com", projectURL: "https://www.djangoproject.com/",
		},
		{
			version: "4.25.7+rhlw.3", release: "rhlw.3", upstream: "4.25.7", published: "2024-02-01T00:00:00Z",
			summary: "A high-level Python web framework", description: "Django is a high-level Python web framework.",
		},
		{
			version: "4.25.70+rhlw.1", release: "rhlw.1", upstream: "4.25.70", published: "2024-03-01T00:00:00Z",
			summary: "must not match 4.25.7",
		},
		{
			version: "4.2", release: "", upstream: "", published: "2023-01-01T00:00:00Z",
		},
		{
			version: "5.0", release: "", upstream: "5.0", published: "2024-01-01T12:00:00Z",
			summary: "Django 5",
		},
	}
	for _, row := range rows {
		versionUUID := uuid.NewString()
		err = db.DB.WithContext(ctx).Exec(`
			INSERT INTO lightwell_package_versions (
				uuid, created_at, updated_at, lightwell_package_uuid, repository_configuration_uuid,
				version, release, upstream_version, published_at, purl, summary, description, license, author, author_email, project_url
			) VALUES (?, NOW(), NOW(), ?, ?, ?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)`,
			versionUUID, pkgUUID, repoConfigUUID, row.version, row.release, row.upstream, row.published,
			row.summary, row.description, row.license, row.author, row.authorEmail, row.projectURL,
		).Error
		s.Require().NoError(err)
		s.T().Cleanup(func() {
			_ = db.DB.Exec(`DELETE FROM lightwell_package_versions WHERE uuid = ?`, versionUUID).Error
		})
	}

	got, err := pkgDao.GetPythonPackageDetail(ctx, repoConfigUUID, "django", "4.25.7")
	s.Require().NoError(err)
	assert.Equal(s.T(), "django", got.Name)
	assert.Equal(s.T(), "4.25.7", got.Version)
	assert.Equal(s.T(), "A high-level Python web framework", got.Summary)
	assert.Equal(s.T(), "Django is a high-level Python web framework.", got.Description)
	assert.Equal(s.T(), "2024-02-01T00:00:00Z", got.LastUpdated)
	assert.Equal(s.T(), "", got.License)
	assert.Equal(s.T(), api.PythonPackageAuthor{}, got.Author)
	assert.Equal(s.T(), "", got.ProjectURL)
	assert.Equal(s.T(), "4.25.7", got.UpstreamVersion)
	assert.Equal(s.T(), []api.ReleaseInfo{
		{Version: "4.25.7", Release: "rhlw.3", CreatedAt: "2024-02-01T00:00:00Z"},
		{Version: "4.25.7", Release: "rhlw.1", CreatedAt: "2024-01-01T00:00:00Z"},
	}, got.Builds)
	assert.Empty(s.T(), got.Distributions)

	got, err = pkgDao.GetPythonPackageDetail(ctx, repoConfigUUID, "django", "5.0")
	s.Require().NoError(err)
	assert.Equal(s.T(), "5.0", got.Version)
	assert.Equal(s.T(), "Django 5", got.Summary)
	assert.Equal(s.T(), []api.ReleaseInfo{
		{Version: "5.0", Release: "", CreatedAt: "2024-01-01T12:00:00Z"},
	}, got.Builds)

	got, err = pkgDao.GetPythonPackageDetail(ctx, repoConfigUUID, "django", "4.2")
	s.Require().NoError(err)
	assert.Equal(s.T(), "4.2", got.Version)
	assert.Equal(s.T(), "", got.UpstreamVersion)
	assert.Equal(s.T(), []api.ReleaseInfo{
		{Version: "4.2", Release: "", CreatedAt: "2023-01-01T00:00:00Z"},
	}, got.Builds)

	_, err = pkgDao.GetPythonPackageDetail(ctx, repoConfigUUID, "django", "9.9.9")
	var missing *ce.DaoError
	s.Require().ErrorAs(err, &missing)
	assert.True(s.T(), missing.NotFound)
}

func (s *LightwellPackageSuite) TestListPythonPackageVersions() {
	ctx, pkgDao := s.dao()
	repoConfigUUID, _ := s.createLightwellRepoWithFeature(
		ctx,
		fmt.Sprintf("python-versions-%s", uuid.NewString()),
		"lightwell-python",
		"remediated",
		config.ContentTypePython,
	)

	pkgUUID := uuid.NewString()
	err := db.DB.WithContext(ctx).Exec(`
		INSERT INTO lightwell_packages (uuid, created_at, updated_at, repository_configuration_uuid, name, package_group)
		VALUES (?, NOW(), NOW(), ?, ?, ?)`,
		pkgUUID, repoConfigUUID, "django", "",
	).Error
	s.Require().NoError(err)
	s.T().Cleanup(func() {
		_ = db.DB.Exec(`DELETE FROM lightwell_packages WHERE uuid = ?`, pkgUUID).Error
	})

	rows := []struct {
		version     string
		release     string
		upstream    string
		publishedAt string
		summary     string
		description string
	}{
		{version: "4.25.7+rhlw.1", release: "rhlw.1", upstream: "4.25.7", publishedAt: "2024-01-01T00:00:00Z", summary: "older django", description: "older description"},
		{version: "4.25.7+rhlw.3", release: "rhlw.3", upstream: "4.25.7", publishedAt: "2024-02-01T00:00:00Z", summary: "Django", description: "newest description"},
		{version: "4.25.70+rhlw.1", release: "rhlw.1", upstream: "4.25.70", publishedAt: "2024-01-15T00:00:00Z", summary: "other"},
		{version: "4.2+rhlw.1", release: "rhlw.1", upstream: "4.2", publishedAt: "2023-06-01T00:00:00Z", summary: "stale"},
		{version: "4.2", publishedAt: "2024-01-01T12:00:00Z"},
	}
	for _, row := range rows {
		versionUUID := uuid.NewString()
		err = db.DB.WithContext(ctx).Exec(`
			INSERT INTO lightwell_package_versions (
				uuid, created_at, updated_at, lightwell_package_uuid, repository_configuration_uuid,
				version, release, upstream_version, published_at, purl, summary, description
			) VALUES (?, NOW(), NOW(), ?, ?, ?, ?, ?, ?, '', ?, ?)`,
			versionUUID, pkgUUID, repoConfigUUID, row.version, row.release, row.upstream, row.publishedAt, row.summary, row.description,
		).Error
		s.Require().NoError(err)
		s.T().Cleanup(func() {
			_ = db.DB.Exec(`DELETE FROM lightwell_package_versions WHERE uuid = ?`, versionUUID).Error
		})
	}

	got, err := pkgDao.ListPythonPackageVersions(ctx, repoConfigUUID, "django")
	s.Require().NoError(err)
	assert.Equal(s.T(), "django", got.Name)
	s.Require().Len(got.Versions, 3)
	assert.Equal(s.T(), "4.25.7", got.Versions[0].Version)
	assert.Equal(s.T(), "4.25.7", got.Versions[0].UpstreamVersion)
	assert.Equal(s.T(), "2024-02-01T00:00:00Z", got.Versions[0].LastUpdated)
	assert.Equal(s.T(), "Django", got.Versions[0].Summary)
	assert.Equal(s.T(), "newest description", got.Versions[0].Description)
	assert.Equal(s.T(), []api.ReleaseInfo{
		{Version: "4.25.7", Release: "rhlw.3", CreatedAt: "2024-02-01T00:00:00Z"},
		{Version: "4.25.7", Release: "rhlw.1", CreatedAt: "2024-01-01T00:00:00Z"},
	}, got.Versions[0].Builds)
	assert.Empty(s.T(), got.Versions[0].Distributions)
	assert.Equal(s.T(), "4.25.70", got.Versions[1].Version)
	s.Require().Len(got.Versions[1].Builds, 1)
	assert.Equal(s.T(), "4.25.70", got.Versions[1].Builds[0].Version)
	assert.Equal(s.T(), "4.2", got.Versions[2].Version)
	assert.Equal(s.T(), "", got.Versions[2].UpstreamVersion)
	assert.Equal(s.T(), "", got.Versions[2].Summary)
	assert.Equal(s.T(), "2024-01-01T12:00:00Z", got.Versions[2].LastUpdated)
	assert.Equal(s.T(), []api.ReleaseInfo{
		{Version: "4.2", Release: "", CreatedAt: "2024-01-01T12:00:00Z"},
		{Version: "4.2", Release: "rhlw.1", CreatedAt: "2023-06-01T00:00:00Z"},
	}, got.Versions[2].Builds)
	assert.Empty(s.T(), got.Versions[2].Distributions)

	_, err = pkgDao.ListPythonPackageVersions(ctx, repoConfigUUID, "missing")
	var daoErr *ce.DaoError
	s.Require().ErrorAs(err, &daoErr)
	assert.True(s.T(), daoErr.NotFound)
}

func (s *LightwellPackageSuite) TestListMavenPackageVersions() {
	ctx, pkgDao := s.dao()
	repoConfigUUID, _ := s.createLightwellRepoWithFeature(
		ctx,
		fmt.Sprintf("maven-versions-%s", uuid.NewString()),
		"lightwell-maven",
		"remediated",
		config.ContentTypeMaven,
	)

	pkgUUID := uuid.NewString()
	err := db.DB.WithContext(ctx).Exec(`
		INSERT INTO lightwell_packages (uuid, created_at, updated_at, repository_configuration_uuid, name, package_group)
		VALUES (?, NOW(), NOW(), ?, ?, ?)`,
		pkgUUID, repoConfigUUID, "lib", "org.example",
	).Error
	s.Require().NoError(err)
	s.T().Cleanup(func() {
		_ = db.DB.Exec(`DELETE FROM lightwell_packages WHERE uuid = ?`, pkgUUID).Error
	})

	rows := []struct {
		version     string
		release     string
		upstream    string
		publishedAt string
		summary     string
		license     string
	}{
		{version: "5.3.18.rhlw-00001", release: "rhlw-00001", upstream: "5.3.18", publishedAt: "2024-03-01T00:00:00Z", summary: "older library", license: "MIT"},
		{version: "5.3.18.rhlw-00003", release: "rhlw-00003", upstream: "5.3.18", publishedAt: "2024-03-03T00:00:00Z", summary: "A library", license: "Apache-2.0"},
		{version: "3.15.0.rhlw-00001", release: "rhlw-00001", upstream: "3.15.0", publishedAt: "2023-06-01T00:00:00Z", summary: "stale", license: "OLD"},
		{version: "3.15.0", publishedAt: "2024-01-01T00:00:00Z"},
	}
	for _, row := range rows {
		versionUUID := uuid.NewString()
		err = db.DB.WithContext(ctx).Exec(`
			INSERT INTO lightwell_package_versions (
				uuid, created_at, updated_at, lightwell_package_uuid, repository_configuration_uuid,
				version, release, upstream_version, published_at, purl, summary, license
			) VALUES (?, NOW(), NOW(), ?, ?, ?, ?, ?, ?, '', ?, ?)`,
			versionUUID, pkgUUID, repoConfigUUID, row.version, row.release, row.upstream, row.publishedAt, row.summary, row.license,
		).Error
		s.Require().NoError(err)
		s.T().Cleanup(func() {
			_ = db.DB.Exec(`DELETE FROM lightwell_package_versions WHERE uuid = ?`, versionUUID).Error
		})
	}

	got, err := pkgDao.ListMavenPackageVersions(ctx, repoConfigUUID, "org.example", "lib")
	s.Require().NoError(err)
	assert.Equal(s.T(), "org.example", got.Group)
	assert.Equal(s.T(), "lib", got.Name)
	s.Require().Len(got.Versions, 2)
	assert.Equal(s.T(), "5.3.18", got.Versions[0].Version)
	assert.Equal(s.T(), "5.3.18", got.Versions[0].UpstreamVersion)
	assert.Equal(s.T(), "2024-03-03T00:00:00Z", got.Versions[0].LastUpdated)
	s.Require().Len(got.Versions[0].Builds, 2)
	assert.Equal(s.T(), "rhlw-00003", got.Versions[0].Builds[0].Release)
	assert.Equal(s.T(), "2024-03-03T00:00:00Z", got.Versions[0].Builds[0].CreatedAt)
	assert.Equal(s.T(), "5.3.18", got.Versions[0].Builds[0].Version)
	assert.Equal(s.T(), "rhlw-00001", got.Versions[0].Builds[1].Release)
	s.Require().NotNil(got.Versions[0].Summary)
	assert.Equal(s.T(), "A library", *got.Versions[0].Summary)
	s.Require().NotNil(got.Versions[0].License)
	assert.Equal(s.T(), "Apache-2.0", *got.Versions[0].License)
	assert.Equal(s.T(), "3.15.0", got.Versions[1].Version)
	assert.Equal(s.T(), "", got.Versions[1].UpstreamVersion)
	assert.Equal(s.T(), "2024-01-01T00:00:00Z", got.Versions[1].LastUpdated)
	s.Require().Len(got.Versions[1].Builds, 2)
	assert.Equal(s.T(), "", got.Versions[1].Builds[0].Release)
	assert.Nil(s.T(), got.Versions[1].Summary)
	assert.Nil(s.T(), got.Versions[1].License)

	_, err = pkgDao.ListMavenPackageVersions(ctx, repoConfigUUID, "org.example", "missing")
	var missing *ce.DaoError
	s.Require().ErrorAs(err, &missing)
	assert.True(s.T(), missing.NotFound)
}

func (s *LightwellPackageSuite) TestGetMavenPackageDetail() {
	ctx, pkgDao := s.dao()
	repoConfigUUID, _ := s.createLightwellRepoWithFeature(
		ctx,
		fmt.Sprintf("maven-detail-%s", uuid.NewString()),
		"lightwell-maven",
		"remediated",
		config.ContentTypeMaven,
	)

	pkgUUID := uuid.NewString()
	err := db.DB.WithContext(ctx).Exec(`
		INSERT INTO lightwell_packages (uuid, created_at, updated_at, repository_configuration_uuid, name, package_group)
		VALUES (?, NOW(), NOW(), ?, ?, ?)`,
		pkgUUID, repoConfigUUID, "lib", "org.example",
	).Error
	s.Require().NoError(err)
	s.T().Cleanup(func() {
		_ = db.DB.Exec(`DELETE FROM lightwell_packages WHERE uuid = ?`, pkgUUID).Error
	})

	rows := []struct {
		version     string
		release     string
		upstream    string
		publishedAt string
		summary     string
	}{
		{version: "5.3.18.rhlw-00001", release: "rhlw-00001", upstream: "5.3.18", publishedAt: "2024-03-01T00:00:00Z", summary: "older library"},
		{version: "5.3.18.rhlw-00003", release: "rhlw-00003", upstream: "5.3.18", publishedAt: "2024-03-03T00:00:00Z", summary: "A library"},
		{version: "5.3.180.rhlw-00003", release: "rhlw-00003", upstream: "5.3.180", publishedAt: "2024-03-04T00:00:00Z"},
		{version: "3.15.0.rhlw-00001", release: "rhlw-00001", upstream: "3.15.0", publishedAt: "2023-06-01T00:00:00Z", summary: "stale"},
		{version: "3.15.0", publishedAt: "2024-01-01T00:00:00Z"},
	}
	for _, row := range rows {
		versionUUID := uuid.NewString()
		err = db.DB.WithContext(ctx).Exec(`
			INSERT INTO lightwell_package_versions (
				uuid, created_at, updated_at, lightwell_package_uuid, repository_configuration_uuid,
				version, release, upstream_version, published_at, purl, summary
			) VALUES (?, NOW(), NOW(), ?, ?, ?, ?, ?, ?, '', ?)`,
			versionUUID, pkgUUID, repoConfigUUID, row.version, row.release, row.upstream, row.publishedAt, row.summary,
		).Error
		s.Require().NoError(err)
		s.T().Cleanup(func() {
			_ = db.DB.Exec(`DELETE FROM lightwell_package_versions WHERE uuid = ?`, versionUUID).Error
		})
	}

	got, err := pkgDao.GetMavenPackageDetail(ctx, repoConfigUUID, "org.example", "lib", "5.3.18")
	s.Require().NoError(err)
	assert.Equal(s.T(), "org.example", got.Group)
	assert.Equal(s.T(), "lib", got.Name)
	assert.Equal(s.T(), "5.3.18", got.Version)
	assert.Equal(s.T(), "5.3.18", got.UpstreamVersion)
	assert.Equal(s.T(), "2024-03-03T00:00:00Z", got.LastUpdated)
	s.Require().Len(got.Builds, 2)
	assert.Equal(s.T(), "rhlw-00003", got.Builds[0].Release)
	assert.Equal(s.T(), "5.3.18", got.Builds[0].Version)
	assert.Equal(s.T(), "rhlw-00001", got.Builds[1].Release)
	s.Require().NotNil(got.Summary)
	assert.Equal(s.T(), "A library", *got.Summary)

	other, err := pkgDao.GetMavenPackageDetail(ctx, repoConfigUUID, "org.example", "lib", "5.3.180")
	s.Require().NoError(err)
	s.Require().Len(other.Builds, 1)
	assert.Equal(s.T(), "5.3.180", other.Builds[0].Version)

	stored, err := pkgDao.GetMavenPackageDetail(ctx, repoConfigUUID, "org.example", "lib", "3.15.0")
	s.Require().NoError(err)
	s.Require().Len(stored.Builds, 2)
	assert.Equal(s.T(), "3.15.0", stored.Builds[0].Version)
	assert.Equal(s.T(), "", stored.Builds[0].Release)
	assert.Equal(s.T(), "", stored.UpstreamVersion)
	assert.Equal(s.T(), "2024-01-01T00:00:00Z", stored.LastUpdated)
	assert.Nil(s.T(), stored.Summary)

	_, err = pkgDao.GetMavenPackageDetail(ctx, repoConfigUUID, "org.example", "lib", "9.9.9")
	var missing *ce.DaoError
	s.Require().ErrorAs(err, &missing)
	assert.True(s.T(), missing.NotFound)
}

func (s *LightwellPackageSuite) TestListNpmPackageVersions() {
	ctx, pkgDao := s.dao()
	repoConfigUUID, _ := s.createLightwellRepoWithFeature(
		ctx,
		fmt.Sprintf("npm-versions-%s", uuid.NewString()),
		"lightwell-npm",
		"remediated",
		config.ContentTypeNpm,
	)

	pkgUUID := uuid.NewString()
	err := db.DB.WithContext(ctx).Exec(`
		INSERT INTO lightwell_packages (uuid, created_at, updated_at, repository_configuration_uuid, name, package_group)
		VALUES (?, NOW(), NOW(), ?, ?, ?)`,
		pkgUUID, repoConfigUUID, "is-odd", "@types",
	).Error
	s.Require().NoError(err)
	s.T().Cleanup(func() {
		_ = db.DB.Exec(`DELETE FROM lightwell_packages WHERE uuid = ?`, pkgUUID).Error
	})

	rows := []struct {
		version     string
		release     string
		upstream    string
		publishedAt string
	}{
		{version: "3.0.0+rhlw.1", release: "rhlw.1", upstream: "3.0.0", publishedAt: "2024-01-01T00:00:00Z"},
		{version: "3.0.0+rhlw.3", release: "rhlw.3", upstream: "3.0.0", publishedAt: "2024-04-01T00:00:00Z"},
		{version: "3.0.10+rhlw.1", release: "rhlw.1", upstream: "3.0.10", publishedAt: "2024-02-01T00:00:00Z"},
		{version: "1.2.3", publishedAt: "2023-12-01T00:00:00Z"},
	}
	for _, row := range rows {
		versionUUID := uuid.NewString()
		err = db.DB.WithContext(ctx).Exec(`
			INSERT INTO lightwell_package_versions (
				uuid, created_at, updated_at, lightwell_package_uuid, repository_configuration_uuid,
				version, release, upstream_version, published_at, purl
			) VALUES (?, NOW(), NOW(), ?, ?, ?, ?, ?, ?, '')`,
			versionUUID, pkgUUID, repoConfigUUID, row.version, row.release, row.upstream, row.publishedAt,
		).Error
		s.Require().NoError(err)
		s.T().Cleanup(func() {
			_ = db.DB.Exec(`DELETE FROM lightwell_package_versions WHERE uuid = ?`, versionUUID).Error
		})
	}

	got, err := pkgDao.ListNpmPackageVersions(ctx, repoConfigUUID, "@types", "is-odd")
	s.Require().NoError(err)
	assert.Equal(s.T(), "@types", got.Scope)
	assert.Equal(s.T(), "is-odd", got.Name)
	s.Require().Len(got.Versions, 3)
	assert.Equal(s.T(), "3.0.0", got.Versions[0].Version)
	assert.Equal(s.T(), "2024-04-01T00:00:00Z", got.Versions[0].LastUpdated)
	assert.Equal(s.T(), "3.0.0", got.Versions[0].UpstreamVersion)
	s.Require().Len(got.Versions[0].Builds, 2)
	assert.Equal(s.T(), "rhlw.3", got.Versions[0].Builds[0].Release)
	assert.Equal(s.T(), "3.0.0", got.Versions[0].Builds[0].Version)
	assert.Equal(s.T(), "rhlw.1", got.Versions[0].Builds[1].Release)
	assert.Equal(s.T(), "3.0.10", got.Versions[1].Version)
	s.Require().Len(got.Versions[1].Builds, 1)
	assert.Equal(s.T(), "1.2.3", got.Versions[2].Version)
	assert.Equal(s.T(), "", got.Versions[2].UpstreamVersion)
	assert.Equal(s.T(), "", got.Versions[2].Builds[0].Release)

	_, err = pkgDao.ListNpmPackageVersions(ctx, repoConfigUUID, "@types", "missing")
	var missing *ce.DaoError
	s.Require().ErrorAs(err, &missing)
	assert.True(s.T(), missing.NotFound)
}

func (s *LightwellPackageSuite) TestGetNpmPackageDetail() {
	ctx, pkgDao := s.dao()
	repoConfigUUID, _ := s.createLightwellRepoWithFeature(
		ctx,
		fmt.Sprintf("npm-detail-%s", uuid.NewString()),
		"lightwell-npm",
		"remediated",
		config.ContentTypeNpm,
	)

	pkgUUID := uuid.NewString()
	err := db.DB.WithContext(ctx).Exec(`
		INSERT INTO lightwell_packages (uuid, created_at, updated_at, repository_configuration_uuid, name, package_group)
		VALUES (?, NOW(), NOW(), ?, ?, ?)`,
		pkgUUID, repoConfigUUID, "is-odd", "@types",
	).Error
	s.Require().NoError(err)
	s.T().Cleanup(func() {
		_ = db.DB.Exec(`DELETE FROM lightwell_packages WHERE uuid = ?`, pkgUUID).Error
	})

	unscopedUUID := uuid.NewString()
	err = db.DB.WithContext(ctx).Exec(`
		INSERT INTO lightwell_packages (uuid, created_at, updated_at, repository_configuration_uuid, name, package_group)
		VALUES (?, NOW(), NOW(), ?, ?, ?)`,
		unscopedUUID, repoConfigUUID, "left-pad", "-",
	).Error
	s.Require().NoError(err)
	s.T().Cleanup(func() {
		_ = db.DB.Exec(`DELETE FROM lightwell_packages WHERE uuid = ?`, unscopedUUID).Error
	})

	rows := []struct {
		pkgUUID     string
		version     string
		release     string
		upstream    string
		publishedAt string
	}{
		{pkgUUID: pkgUUID, version: "3.0.0+rhlw.1", release: "rhlw.1", upstream: "3.0.0", publishedAt: "2024-01-01T00:00:00Z"},
		{pkgUUID: pkgUUID, version: "3.0.0+rhlw.3", release: "rhlw.3", upstream: "3.0.0", publishedAt: "2024-04-01T00:00:00Z"},
		{pkgUUID: pkgUUID, version: "3.0.10+rhlw.1", release: "rhlw.1", upstream: "3.0.10", publishedAt: "2024-02-01T00:00:00Z"},
		{pkgUUID: pkgUUID, version: "1.2.3", publishedAt: "2023-12-01T00:00:00Z"},
		{pkgUUID: unscopedUUID, version: "1.0.0", upstream: "1.0.0", publishedAt: "2024-05-01T00:00:00Z"},
	}
	for _, row := range rows {
		versionUUID := uuid.NewString()
		err = db.DB.WithContext(ctx).Exec(`
			INSERT INTO lightwell_package_versions (
				uuid, created_at, updated_at, lightwell_package_uuid, repository_configuration_uuid,
				version, release, upstream_version, published_at, purl
			) VALUES (?, NOW(), NOW(), ?, ?, ?, ?, ?, ?, '')`,
			versionUUID, row.pkgUUID, repoConfigUUID, row.version, row.release, row.upstream, row.publishedAt,
		).Error
		s.Require().NoError(err)
		s.T().Cleanup(func() {
			_ = db.DB.Exec(`DELETE FROM lightwell_package_versions WHERE uuid = ?`, versionUUID).Error
		})
	}

	got, err := pkgDao.GetNpmPackageDetail(ctx, repoConfigUUID, "@types", "is-odd", "3.0.0")
	s.Require().NoError(err)
	assert.Equal(s.T(), "@types", got.Scope)
	assert.Equal(s.T(), "is-odd", got.Name)
	assert.Equal(s.T(), "3.0.0", got.Version)
	assert.Equal(s.T(), "2024-04-01T00:00:00Z", got.LastUpdated)
	assert.Equal(s.T(), "3.0.0", got.UpstreamVersion)
	s.Require().Len(got.Builds, 2)
	assert.Equal(s.T(), "rhlw.3", got.Builds[0].Release)
	assert.Equal(s.T(), "3.0.0", got.Builds[0].Version)
	assert.Equal(s.T(), "rhlw.1", got.Builds[1].Release)
	assert.Empty(s.T(), got.Tarball.Filename)

	other, err := pkgDao.GetNpmPackageDetail(ctx, repoConfigUUID, "@types", "is-odd", "3.0.10")
	s.Require().NoError(err)
	s.Require().Len(other.Builds, 1)
	assert.Equal(s.T(), "3.0.10", other.Builds[0].Version)

	stored, err := pkgDao.GetNpmPackageDetail(ctx, repoConfigUUID, "@types", "is-odd", "1.2.3")
	s.Require().NoError(err)
	s.Require().Len(stored.Builds, 1)
	assert.Equal(s.T(), "1.2.3", stored.Builds[0].Version)
	assert.Equal(s.T(), "", stored.Builds[0].Release)
	assert.Equal(s.T(), "", stored.UpstreamVersion)

	unscoped, err := pkgDao.GetNpmPackageDetail(ctx, repoConfigUUID, "-", "left-pad", "1.0.0")
	s.Require().NoError(err)
	assert.Equal(s.T(), "-", unscoped.Scope)
	assert.Equal(s.T(), "left-pad", unscoped.Name)
	s.Require().Len(unscoped.Builds, 1)

	_, err = pkgDao.GetNpmPackageDetail(ctx, repoConfigUUID, "@types", "is-odd", "9.9.9")
	var missing *ce.DaoError
	s.Require().ErrorAs(err, &missing)
	assert.True(s.T(), missing.NotFound)
}

func TestRepositoryPackageItemUpstreamVersionsMatchVersions(t *testing.T) {
	item := repositoryPackageItem(
		"lib-b",
		"org.example",
		[]string{"1.0.0.rhlw-00001", "1.0.0.rhlw-00003", "2.0.0"},
		[]string{"rhlw-00001", "rhlw-00003", ""},
		[]string{"2024-01-01T00:00:00Z", "2024-03-01T00:00:00Z", "2024-02-01T00:00:00Z"},
		[]string{"1.0.0", "1.0.0", "2.0.0"},
	)
	assert.Equal(t, []string{"1.0.0", "2.0.0"}, item.Versions)
	assert.Equal(t, item.Versions, item.UpstreamVersions)
	assert.Equal(t, []api.ReleaseInfo{
		{Version: "1.0.0", Release: "rhlw-00003", CreatedAt: "2024-03-01T00:00:00Z"},
		{Version: "2.0.0", Release: "", CreatedAt: "2024-02-01T00:00:00Z"},
	}, item.LatestReleases)

	empty := repositoryPackageItem("aaa-empty", "aaa", nil, nil, nil, nil)
	assert.Empty(t, empty.Versions)
	assert.Empty(t, empty.UpstreamVersions)
	assert.NotNil(t, empty.UpstreamVersions)

	// Rows imported before the split store the public version and an empty upstream.
	fallback := repositoryPackageItem("legacy", "", []string{"5.3.18.lw-1"}, []string{""}, []string{"2024-01-01T00:00:00Z"}, []string{""})
	assert.Equal(t, []string{"5.3.18.lw-1"}, fallback.Versions)
	assert.Equal(t, fallback.Versions, fallback.UpstreamVersions)
}

func (s *LightwellPackageSuite) TestListRepositoryPackages() {
	ctx, pkgDao := s.dao()
	repoConfigUUID, _ := s.createLightwellRepoWithFeature(
		ctx,
		fmt.Sprintf("repo-packages-%s", uuid.NewString()),
		"lightwell-maven",
		"remediated",
		config.ContentTypeMaven,
	)

	type versionRow struct {
		version, release, upstream, published string
	}
	packages := []struct {
		group, name string
		versions    []versionRow
	}{
		{group: "aaa", name: "aaa-empty"},
		{group: "", name: "django", versions: []versionRow{
			{version: "4.25.7+rhlw.1", release: "rhlw.1", upstream: "4.25.7", published: "2024-01-01T00:00:00Z"},
			{version: "4.25.7+rhlw.3", release: "rhlw.3", upstream: "4.25.7", published: "2024-04-01T00:00:00Z"},
		}},
		{group: "@types", name: "is-odd", versions: []versionRow{
			{version: "3.0.0", upstream: "3.0.0", published: "2024-05-01T00:00:00Z"},
		}},
		{group: "org.example", name: "lib-a", versions: []versionRow{
			{version: "1.0.0", upstream: "1.0.0", published: "2024-06-01T00:00:00Z"},
		}},
		{group: "org.example", name: "lib-b", versions: []versionRow{
			{version: "1.0.0.rhlw-00001", release: "rhlw-00001", upstream: "1.0.0", published: "2024-01-01T00:00:00Z"},
			{version: "1.0.0.rhlw-00003", release: "rhlw-00003", upstream: "1.0.0", published: "2024-03-01T00:00:00Z"},
			{version: "2.0.0", upstream: "2.0.0", published: "2024-02-01T00:00:00Z"},
		}},
	}
	for _, pkg := range packages {
		pkgUUID := uuid.NewString()
		err := db.DB.WithContext(ctx).Exec(`
			INSERT INTO lightwell_packages (uuid, created_at, updated_at, repository_configuration_uuid, name, package_group)
			VALUES (?, NOW(), NOW(), ?, ?, ?)`,
			pkgUUID, repoConfigUUID, pkg.name, pkg.group,
		).Error
		s.Require().NoError(err)
		s.T().Cleanup(func() {
			_ = db.DB.Exec(`DELETE FROM lightwell_packages WHERE uuid = ?`, pkgUUID).Error
		})
		for _, version := range pkg.versions {
			versionUUID := uuid.NewString()
			err = db.DB.WithContext(ctx).Exec(`
				INSERT INTO lightwell_package_versions (
					uuid, created_at, updated_at, lightwell_package_uuid, repository_configuration_uuid,
					version, release, upstream_version, published_at, purl
				) VALUES (?, NOW(), NOW(), ?, ?, ?, ?, ?, ?, '')`,
				versionUUID, pkgUUID, repoConfigUUID, version.version, version.release, version.upstream, version.published,
			).Error
			s.Require().NoError(err)
			s.T().Cleanup(func() {
				_ = db.DB.Exec(`DELETE FROM lightwell_package_versions WHERE uuid = ?`, versionUUID).Error
			})
		}
	}

	names := func(resp api.PackageResponse) []string {
		out := make([]string, len(resp.Results))
		for i, item := range resp.Results {
			out[i] = item.Name
		}
		return out
	}

	got, err := pkgDao.ListRepositoryPackages(ctx, repoConfigUUID, "", "group/name", "asc", 100, 0)
	s.Require().NoError(err)
	assert.Equal(s.T(), 5, got.Total)
	assert.Equal(s.T(), []string{"django", "aaa-empty", "lib-a", "lib-b", "is-odd"}, names(got))

	byName := map[string]api.PackageItem{}
	for _, item := range got.Results {
		byName[item.Name] = item
	}
	assert.Equal(s.T(), []string{}, byName["aaa-empty"].Versions)
	assert.Equal(s.T(), []api.ReleaseInfo{}, byName["aaa-empty"].LatestReleases)
	assert.Equal(s.T(), []string{"4.25.7"}, byName["django"].Versions)
	for name, item := range byName {
		assert.Equal(s.T(), item.Versions, item.UpstreamVersions, name)
	}
	assert.Equal(s.T(), []api.ReleaseInfo{
		{Version: "4.25.7", Release: "rhlw.3", CreatedAt: "2024-04-01T00:00:00Z"},
	}, byName["django"].LatestReleases)
	assert.Equal(s.T(), []string{"1.0.0", "2.0.0"}, byName["lib-b"].Versions)
	assert.Equal(s.T(), []api.ReleaseInfo{
		{Version: "1.0.0", Release: "rhlw-00003", CreatedAt: "2024-03-01T00:00:00Z"},
		{Version: "2.0.0", Release: "", CreatedAt: "2024-02-01T00:00:00Z"},
	}, byName["lib-b"].LatestReleases)

	got, err = pkgDao.ListRepositoryPackages(ctx, repoConfigUUID, "", "name", "asc", 100, 0)
	s.Require().NoError(err)
	assert.Equal(s.T(), []string{"aaa-empty", "django", "is-odd", "lib-a", "lib-b"}, names(got))

	got, err = pkgDao.ListRepositoryPackages(ctx, repoConfigUUID, "", "name", "desc", 100, 0)
	s.Require().NoError(err)
	assert.Equal(s.T(), []string{"lib-b", "lib-a", "is-odd", "django", "aaa-empty"}, names(got))

	got, err = pkgDao.ListRepositoryPackages(ctx, repoConfigUUID, "", "last_updated", "desc", 100, 0)
	s.Require().NoError(err)
	assert.Equal(s.T(), []string{"lib-a", "is-odd", "django", "lib-b", "aaa-empty"}, names(got))

	got, err = pkgDao.ListRepositoryPackages(ctx, repoConfigUUID, "", "last_updated", "asc", 100, 0)
	s.Require().NoError(err)
	assert.Equal(s.T(), []string{"lib-b", "django", "is-odd", "lib-a", "aaa-empty"}, names(got))

	got, err = pkgDao.ListRepositoryPackages(ctx, repoConfigUUID, "lib-", "group/name", "asc", 100, 0)
	s.Require().NoError(err)
	assert.Equal(s.T(), []string{"lib-a", "lib-b"}, names(got))

	got, err = pkgDao.ListRepositoryPackages(ctx, repoConfigUUID, "org.example:lib-a", "group/name", "asc", 100, 0)
	s.Require().NoError(err)
	assert.Equal(s.T(), []string{"lib-a"}, names(got))

	got, err = pkgDao.ListRepositoryPackages(ctx, repoConfigUUID, "@types/", "group/name", "asc", 100, 0)
	s.Require().NoError(err)
	assert.Equal(s.T(), []string{"is-odd"}, names(got))

	got, err = pkgDao.ListRepositoryPackages(ctx, repoConfigUUID, "DJANGO", "name", "asc", 1, 0)
	s.Require().NoError(err)
	assert.Equal(s.T(), 1, got.Total)
	assert.Equal(s.T(), []string{"django"}, names(got))

	got, err = pkgDao.ListRepositoryPackages(ctx, repoConfigUUID, "", "name", "asc", 1, 1)
	s.Require().NoError(err)
	assert.Equal(s.T(), 5, got.Total)
	assert.Equal(s.T(), 1, got.Limit)
	assert.Equal(s.T(), 1, got.Offset)
	assert.Equal(s.T(), []string{"django"}, names(got))

	_, err = pkgDao.ListRepositoryPackages(ctx, "not-a-uuid", "", "name", "asc", 10, 0)
	var daoErr *ce.DaoError
	s.Require().ErrorAs(err, &daoErr)
	assert.True(s.T(), daoErr.BadValidation)
}
