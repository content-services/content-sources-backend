package sync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/jira_client"
	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/tang/pkg/tangy"
	"github.com/stretchr/testify/assert"
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

	packages, byIssue := collectBreadcrumbPackages([]jira_client.JiraIssue{validation, nonValidation, discarded})

	expectedPackage := PublicationPackage{Ecosystem: "maven", Namespace: "org.example", Name: "demo"}
	assert.Equal(t, map[PublicationPackage][]string{expectedPackage: {"1.0.0.rhlw-00001", "2.0.0-rhlw-00002"}}, packages)
	assert.Len(t, byIssue["LTWL-1"], 2)
	assert.NotContains(t, byIssue, "LTWL-2")
	assert.NotContains(t, byIssue, "LTWL-3")
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
		{Name: "ignored", PublishedDistBasePath: "java/validated", ContentType: "maven"},
		{Name: "java remediated", PublishedDistBasePath: "java/remediated", ContentType: "maven"},
		{Name: "java predisclosure", PublishedDistBasePath: "java/predisclosure", ContentType: "maven"},
		{Name: "python remediated", PublishedDistBasePath: "python/remediated", ContentType: "python"},
	})
	resolver := pulp_client.NewMockPulpClient(t)
	expectPublicationRepository(resolver, ctx, "java/remediated")
	expectPublicationRepository(resolver, ctx, "java/predisclosure")
	expectPublicationRepository(resolver, ctx, "python/remediated")
	tang := tangy.NewMockTangy(t)
	tang.On("MavenVersionsList", ctx, "java/remediated-href", "org.example", "demo", "", tangy.PageOptions{Limit: 1000}).
		Return(tangy.MavenVersionsResponse{}, nil).Once()
	tang.On("MavenVersionsList", ctx, "java/predisclosure-href", "org.example", "demo", "", tangy.PageOptions{Limit: 1000}).Return(tangy.MavenVersionsResponse{
		Results: []tangy.MavenVersionsItem{{
			GroupID: "org.example", ArtifactID: "demo", Version: "1.2.3",
			Builds: []tangy.MavenBuildInfo{{Version: "1.2.3", Release: "rhlw-00002"}},
		}},
		Total: 1,
	}, nil).Once()
	tang.On("PythonPackageVersionsGet", ctx, "python/remediated-href", "demo-pkg").
		Return([]tangy.PythonPackageDetail{{Version: "5.0-rhlw-00001"}}, nil).Once()
	tang.On("PythonPackageVersionsGet", ctx, "python/remediated-href", "plain").
		Return(nil, fmt.Errorf("%w: plain", tangy.ErrPythonPackageNotFound)).Once()

	maven := PublicationPackage{Ecosystem: "maven", Namespace: "org.example", Name: "demo"}
	python := PublicationPackage{Ecosystem: "pypi", Name: "Demo_Pkg"}
	missingPython := PublicationPackage{Ecosystem: "pypi", Name: "plain"}
	verifier := NewPublicationPackageVerifier(repositories, resolver, tang)

	confirmed, warnings := verifier.VerifyPublished(ctx, map[PublicationPackage][]string{
		maven:         {"1.2.3.rhlw-00002", "9.9.9"},
		python:        {"5.0-rhlw-00001"},
		missingPython: {"1.0.0"},
	})

	assert.Empty(t, warnings)
	assert.Equal(t, []string{"1.2.3.rhlw-00002"}, confirmed[maven])
	assert.Equal(t, []string{"5.0-rhlw-00001"}, confirmed[python])
	assert.NotContains(t, confirmed, missingPython)
	resolver.AssertNotCalled(t, "ResolveRepositoryFromBasePath", ctx, "java/validated")
}

func TestPublicationPackageVerifierKeepsPartialResultsAndWarnings(t *testing.T) {
	ctx := context.Background()
	repositories := mockPublicationRepositories(t, ctx, []api.RepositoryResponse{
		{PublishedDistBasePath: "java/remediated", ContentType: "maven"},
		{PublishedDistBasePath: "java/predisclosure", ContentType: "maven"},
	})
	resolver := pulp_client.NewMockPulpClient(t)
	expectPublicationRepository(resolver, ctx, "java/remediated")
	expectPublicationRepository(resolver, ctx, "java/predisclosure")
	tang := tangy.NewMockTangy(t)
	tang.On("MavenVersionsList", ctx, "java/remediated-href", "org.example", "demo", "", tangy.PageOptions{Limit: 1000}).
		Return(tangy.MavenVersionsResponse{}, errors.New("Tang unavailable")).Once()
	tang.On("MavenVersionsList", ctx, "java/predisclosure-href", "org.example", "demo", "", tangy.PageOptions{Limit: 1000}).Return(tangy.MavenVersionsResponse{
		Results: []tangy.MavenVersionsItem{{Version: "1.0.0"}}, Total: 1,
	}, nil).Once()
	pkg := PublicationPackage{Ecosystem: "maven", Namespace: "org.example", Name: "demo"}

	confirmed, warnings := NewPublicationPackageVerifier(repositories, resolver, tang).
		VerifyPublished(ctx, map[PublicationPackage][]string{pkg: {"1.0.0"}})

	assert.Equal(t, []string{"1.0.0"}, confirmed[pkg])
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "Tang unavailable")
}

func TestPublicationPackageVerifierChecksMavenVersionsBeyondFirstPage(t *testing.T) {
	ctx := context.Background()
	tang := tangy.NewMockTangy(t)
	firstPage := make([]tangy.MavenVersionsItem, 1000)
	for i := range firstPage {
		firstPage[i].Version = fmt.Sprintf("1.0.%d", i)
	}
	tang.On("MavenVersionsList", ctx, "java/remediated-href", "org.example", "demo", "", tangy.PageOptions{Limit: 1000}).
		Return(tangy.MavenVersionsResponse{Results: firstPage, Total: 1001}, nil).Once()
	tang.On("MavenVersionsList", ctx, "java/remediated-href", "org.example", "demo", "", tangy.PageOptions{Offset: 1000, Limit: 1000}).
		Return(tangy.MavenVersionsResponse{
			Results: []tangy.MavenVersionsItem{{Version: "2.0.0"}}, Total: 1001,
		}, nil).Once()

	verifier := &publicationPackageVerifier{tang: tang}
	available, err := verifier.packageVersions(ctx, "java/remediated-href", PublicationPackage{
		Ecosystem: "maven", Namespace: "org.example", Name: "demo",
	})

	require.NoError(t, err)
	assert.True(t, available["2.0.0"])
}

func TestPublicationPackageVerifierReportsRepositoryFailures(t *testing.T) {
	pkg := PublicationPackage{Ecosystem: "pypi", Name: "demo"}

	t.Run("list repositories", func(t *testing.T) {
		ctx := context.Background()
		repositories := dao.NewMockRepositoryConfigDao(t)
		repositories.On("InternalOnly_FetchRepoConfigForOrg", ctx, config.LightwellOrg).
			Return(nil, errors.New("database unavailable")).Once()
		confirmed, warnings := NewPublicationPackageVerifier(repositories, pulp_client.NewMockPulpClient(t), tangy.NewMockTangy(t)).
			VerifyPublished(ctx, map[PublicationPackage][]string{pkg: {"1.0.0"}})
		assert.Empty(t, confirmed)
		require.Len(t, warnings, 1)
		assert.Contains(t, warnings[0], "database unavailable")
	})

	t.Run("resolve one repository", func(t *testing.T) {
		ctx := context.Background()
		repositories := mockPublicationRepositories(t, ctx, []api.RepositoryResponse{{
			PublishedDistBasePath: "python/remediated", ContentType: "python",
		}})
		resolver := pulp_client.NewMockPulpClient(t)
		resolver.On("ResolveRepositoryFromBasePath", ctx, "python/remediated").
			Return(nil, errors.New("Pulp unavailable")).Once()
		confirmed, warnings := NewPublicationPackageVerifier(repositories, resolver, tangy.NewMockTangy(t)).
			VerifyPublished(ctx, map[PublicationPackage][]string{pkg: {"1.0.0"}})
		assert.Empty(t, confirmed)
		require.Len(t, warnings, 1)
		assert.Contains(t, warnings[0], "Pulp unavailable")
	})
}

func mockPublicationRepositories(t *testing.T, ctx context.Context, repos []api.RepositoryResponse) *dao.MockRepositoryConfigDao {
	t.Helper()
	repositories := dao.NewMockRepositoryConfigDao(t)
	repositories.On("InternalOnly_FetchRepoConfigForOrg", ctx, config.LightwellOrg).Return(repos, nil).Once()
	return repositories
}

func expectPublicationRepository(resolver *pulp_client.MockPulpClient, ctx context.Context, basePath string) {
	href := basePath + "-href"
	resolver.On("ResolveRepositoryFromBasePath", ctx, basePath).Return(&href, nil).Once()
}
