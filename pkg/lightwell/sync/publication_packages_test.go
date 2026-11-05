package sync

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/jira_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestFixedVersionPackages(t *testing.T) {
	tests := []struct {
		name        string
		description json.RawMessage
		expected    []publicationPackageVersion
	}{
		{
			name: "plain text backport and novel sections",
			description: jsonString(t, `Component PURL: pkg:maven/ignored/component@9.9.9
Lightwell Fixed Versions (backport fixes):
- pkg:maven/org.example/demo@1.2.3.rhlw-00001
- pkg:maven/org.example/demo@1.2.3.rhlw-00002
Lightwell Fixed Versions (novel fixes):
- pkg:maven/org.example/demo@1.2.3.rhlw-00001-n-00001`),
			expected: []publicationPackageVersion{
				{Package: PublicationPackage{Ecosystem: "maven", Namespace: "org.example", Name: "demo"}, Version: "1.2.3.rhlw-00001"},
				{Package: PublicationPackage{Ecosystem: "maven", Namespace: "org.example", Name: "demo"}, Version: "1.2.3.rhlw-00002"},
				{Package: PublicationPackage{Ecosystem: "maven", Namespace: "org.example", Name: "demo"}, Version: "1.2.3.rhlw-00001-n-00001"},
			},
		},
		{
			name: "ADF split heading",
			description: json.RawMessage(`{
				"type":"doc","content":[
					{"type":"paragraph","content":[{"type":"text","text":"pkg:maven/ignored/component@9.9.9"}]},
					{"type":"paragraph","content":[
						{"type":"text","text":"Lightwell Fixed "},
						{"type":"text","text":"Versions"},
						{"type":"text","text":":"},
						{"type":"hardBreak"},
						{"type":"text","text":"pkg:pypi/django@5.0-rhlw-00001"}
					]}
				]}`),
			expected: []publicationPackageVersion{
				{Package: PublicationPackage{Ecosystem: "pypi", Name: "django"}, Version: "5.0-rhlw-00001"},
			},
		},
		{
			name: "duplicates malformed PURLs and missing versions",
			description: jsonString(t, `LIGHTWELL FIXED VERSIONS:
[pkg:maven/org.example/demo@1.0.0.rhlw-00001]
pkg:maven/org.example/demo@1.0.0.rhlw-00001
pkg:maven/org.example/demo@1.0.0
pkg:pypi/demo@2.0.0
pkg:maven/org.example/demo
pkg:npm/%40types/is-odd@3.0.0
pkg:not a purl`),
			expected: []publicationPackageVersion{
				{Package: PublicationPackage{Ecosystem: "maven", Namespace: "org.example", Name: "demo"}, Version: "1.0.0.rhlw-00001"},
			},
		},
		{
			name:        "missing heading",
			description: jsonString(t, "pkg:maven/org.example/demo@1.0.0"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expected, fixedVersionPackages(test.description))
		})
	}
}

func TestCollectBreadcrumbPackagesFiltersIssuesAndDeduplicatesLookups(t *testing.T) {
	validation := breadcrumbJiraIssue(t, "LTWL-1", "Closed", `Lightwell Fixed Versions:
pkg:maven/org.example/demo@1.0.0.rhlw-00001
pkg:maven/org.example/demo@2.0.0-rhlw-00002
pkg:maven/org.example/demo@1.0.0
pkg:maven/org.example/demo@1.0.0.rhlw-00001`)
	nonValidation := breadcrumbJiraIssue(t, "LTWL-2", "In Progress", "Lightwell Fixed Versions: pkg:pypi/django@5.0")
	discarded := breadcrumbJiraIssue(t, "LTWL-3", "Closed", "Lightwell Fixed Versions: pkg:pypi/demo@1.0.0")
	discarded.Fields["resolution"] = json.RawMessage(`{"name":"Duplicate"}`)
	closed := breadcrumbJiraIssue(t, "LTWL-4", "Closed", "Lightwell Fixed Versions: pkg:pypi/demo@3.0.0")
	closed.Fields["resolution"] = json.RawMessage(`{"name":"Won't do"}`)

	packages, byIssue := collectBreadcrumbPackages([]jira_client.JiraIssue{validation, nonValidation, discarded, closed})

	expectedPackage := PublicationPackage{Ecosystem: "maven", Namespace: "org.example", Name: "demo"}
	assert.Equal(t, map[PublicationPackage][]string{expectedPackage: {"1.0.0.rhlw-00001", "2.0.0-rhlw-00002"}}, packages)
	assert.Len(t, byIssue["LTWL-1"], 2)
	assert.NotContains(t, byIssue, "LTWL-2")
	assert.NotContains(t, byIssue, "LTWL-3")
	assert.NotContains(t, byIssue, "LTWL-4")
}

func jsonString(t *testing.T, value string) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	return raw
}

func breadcrumbJiraIssue(t *testing.T, key, status, description string) jira_client.JiraIssue {
	t.Helper()
	issue := validJiraIssue(key)
	statusJSON, err := json.Marshal(map[string]string{"name": status})
	require.NoError(t, err)
	issue.Fields["status"] = statusJSON
	issue.Fields["description"] = jsonString(t, description)
	return issue
}

func TestPublicationPackageVerifierChecksAllSupportedPackageTypes(t *testing.T) {
	ctx := context.Background()
	repositories := mockPublicationRepositories(t, ctx, []api.RepositoryResponse{
		importedPublicationRepo("ignored", "java/validated", "maven"),
		importedPublicationRepo("java-remediated", "java/remediated", "maven"),
		importedPublicationRepo("java-predisclosure", "java/predisclosure", "maven"),
		importedPublicationRepo("python-remediated", "python/remediated", "python"),
	})
	packages := dao.NewMockLightwellPackageDao(t)
	expectStoredVersions(packages, ctx, "java-remediated", "org.example", "demo", nil, nil)
	expectStoredVersions(packages, ctx, "java-predisclosure", "org.example", "demo", []string{"1.2.3.rhlw-00002"}, nil)
	expectStoredVersions(packages, ctx, "python-remediated", "", "demo-pkg", []string{"5.0-rhlw-00001"}, nil)
	expectStoredVersions(packages, ctx, "python-remediated", "", "plain", nil, nil)

	maven := PublicationPackage{Ecosystem: "maven", Namespace: "org.example", Name: "demo"}
	python := PublicationPackage{Ecosystem: "pypi", Name: "Demo_Pkg"}
	missingPython := PublicationPackage{Ecosystem: "pypi", Name: "plain"}
	verifier := NewPublicationPackageVerifier(repositories, packages)

	confirmed, warnings := verifier.VerifyPublished(ctx, map[PublicationPackage][]string{
		maven:         {"1.2.3.rhlw-00002", "9.9.9"},
		python:        {"5.0-rhlw-00001"},
		missingPython: {"1.0.0"},
	})

	assert.Empty(t, warnings)
	assert.Equal(t, []string{"1.2.3.rhlw-00002"}, confirmed[maven])
	assert.Equal(t, []string{"5.0-rhlw-00001"}, confirmed[python])
	assert.NotContains(t, confirmed, missingPython)
	packages.AssertNotCalled(t, "InternalOnlyListStoredPackageVersions", mock.Anything, "ignored", mock.Anything, mock.Anything)
}

func TestPublicationPackageVerifierKeepsPartialResultsAndWarnings(t *testing.T) {
	ctx := context.Background()
	repositories := mockPublicationRepositories(t, ctx, []api.RepositoryResponse{
		importedPublicationRepo("java-remediated", "java/remediated", "maven"),
		importedPublicationRepo("java-predisclosure", "java/predisclosure", "maven"),
	})
	packages := dao.NewMockLightwellPackageDao(t)
	expectStoredVersions(packages, ctx, "java-remediated", "org.example", "demo", nil, errors.New("mirror unavailable"))
	expectStoredVersions(packages, ctx, "java-predisclosure", "org.example", "demo", []string{"1.0.0"}, nil)
	pkg := PublicationPackage{Ecosystem: "maven", Namespace: "org.example", Name: "demo"}

	confirmed, warnings := NewPublicationPackageVerifier(repositories, packages).
		VerifyPublished(ctx, map[PublicationPackage][]string{pkg: {"1.0.0"}})

	assert.Equal(t, []string{"1.0.0"}, confirmed[pkg])
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "mirror unavailable")
}

func TestPublicationPackageVerifierSkipsUnimportedRepositories(t *testing.T) {
	ctx := context.Background()
	repositories := mockPublicationRepositories(t, ctx, []api.RepositoryResponse{
		{UUID: "java-remediated", PublishedDistBasePath: "java/remediated", ContentType: "maven"},
		importedPublicationRepo("java-predisclosure", "java/predisclosure", "maven"),
	})
	packages := dao.NewMockLightwellPackageDao(t)
	expectStoredVersions(packages, ctx, "java-predisclosure", "org.example", "demo", []string{"1.0.0"}, nil)
	pkg := PublicationPackage{Ecosystem: "maven", Namespace: "org.example", Name: "demo"}

	confirmed, warnings := NewPublicationPackageVerifier(repositories, packages).
		VerifyPublished(ctx, map[PublicationPackage][]string{pkg: {"1.0.0"}})

	assert.Equal(t, []string{"1.0.0"}, confirmed[pkg])
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "java/remediated")
	assert.Contains(t, warnings[0], "package mirror has not been imported")
	packages.AssertNotCalled(t, "InternalOnlyListStoredPackageVersions", mock.Anything, "java-remediated", mock.Anything, mock.Anything)
}

func TestPublicationPackageVerifierReportsRepositoryFailures(t *testing.T) {
	pkg := PublicationPackage{Ecosystem: "pypi", Name: "demo"}

	t.Run("list repositories", func(t *testing.T) {
		ctx := context.Background()
		repositories := dao.NewMockRepositoryConfigDao(t)
		repositories.On("InternalOnly_FetchRepoConfigForOrg", ctx, config.LightwellOrg).
			Return(nil, errors.New("database unavailable")).Once()
		confirmed, warnings := NewPublicationPackageVerifier(repositories, dao.NewMockLightwellPackageDao(t)).
			VerifyPublished(ctx, map[PublicationPackage][]string{pkg: {"1.0.0"}})
		assert.Empty(t, confirmed)
		require.Len(t, warnings, 1)
		assert.Contains(t, warnings[0], "database unavailable")
	})

	t.Run("repository has not been imported", func(t *testing.T) {
		ctx := context.Background()
		repositories := mockPublicationRepositories(t, ctx, []api.RepositoryResponse{{
			UUID: "python-remediated", PublishedDistBasePath: "python/remediated", ContentType: "python",
		}})
		packages := dao.NewMockLightwellPackageDao(t)
		confirmed, warnings := NewPublicationPackageVerifier(repositories, packages).
			VerifyPublished(ctx, map[PublicationPackage][]string{pkg: {"1.0.0"}})
		assert.Empty(t, confirmed)
		require.Len(t, warnings, 1)
		assert.Contains(t, warnings[0], "package mirror has not been imported")
		packages.AssertNotCalled(t, "InternalOnlyListStoredPackageVersions", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})
}

func mockPublicationRepositories(t *testing.T, ctx context.Context, repos []api.RepositoryResponse) *dao.MockRepositoryConfigDao {
	t.Helper()
	repositories := dao.NewMockRepositoryConfigDao(t)
	repositories.On("InternalOnly_FetchRepoConfigForOrg", ctx, config.LightwellOrg).Return(repos, nil).Once()
	return repositories
}

func importedPublicationRepo(uuid, basePath, contentType string) api.RepositoryResponse {
	return api.RepositoryResponse{
		UUID:                        uuid,
		PublishedDistBasePath:       basePath,
		ContentType:                 contentType,
		LastImportRepositoryVersion: "/versions/1/",
	}
}

func expectStoredVersions(packages *dao.MockLightwellPackageDao, ctx context.Context, repoUUID, group, name string, versions []string, err error) {
	packages.On("InternalOnlyListStoredPackageVersions", ctx, repoUUID, group, name).Return(versions, err).Once()
}
