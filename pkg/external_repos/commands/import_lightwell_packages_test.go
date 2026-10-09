package commands

import (
	"context"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/lightwell/rhlw"
	"github.com/content-services/tang/pkg/tangy"
	zest "github.com/content-services/zest/release/v2026"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestShouldImport(t *testing.T) {
	v := "/versions/5/"
	assert.True(t, shouldImport(&v, "/versions/4/", false))   // changed
	assert.False(t, shouldImport(&v, "/versions/5/", false))  // unchanged
	assert.True(t, shouldImport(&v, "/versions/5/", true))    // force
	assert.False(t, shouldImport(nil, "/versions/5/", false)) // nil current -> nothing to import
}

func TestMapMavenPackageInputs(t *testing.T) {
	resp := zest.PaginatedMavenRepositoryPackageListResponse{
		Results: []zest.MavenRepositoryPackageResponse{{
			GroupId:    "org.apache",
			ArtifactId: "commons",
			Versions:   []string{"1.0", "2.0"},
			LatestReleases: []zest.MavenPackageReleaseResponse{
				{Version: "2.0", Release: "ga", CreatedAt: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)},
			},
		}},
	}
	got := mapMavenPackageInputs(resp)
	assert.Len(t, got, 1)
	assert.Equal(t, "commons", got[0].Name)
	assert.Equal(t, "org.apache", got[0].Group)
	assert.Len(t, got[0].Versions, 2)
	// version 2.0 carries release + published_at + purl; 1.0 has empty release/published_at
	byVer := make(map[string]dao.LightwellPackageVersionInput)
	for _, v := range got[0].Versions {
		byVer[v.Version] = v
	}
	assert.Equal(t, "ga", byVer["2.0"].Release)
	assert.Equal(t, "2020-01-01T00:00:00Z", byVer["2.0"].PublishedAt)
	assert.Equal(t, "pkg:maven/org.apache/commons@2.0", byVer["2.0"].Purl)
	assert.Equal(t, "", byVer["1.0"].Release)
}

func TestMapPythonPackageInputs(t *testing.T) {
	resp := tangy.PythonPackageDetailListResponse{
		Results: []tangy.PythonPackageDetailListItem{{
			NameNormalized: "requests",
			Versions: []tangy.PythonPackageVersionDetail{{
				Version:           "1.2.3+rhlw.1",
				LastUpdated:       "2021-01-01T00:00:00Z",
				Summary:           "HTTP library",
				Description:       "HTTP library for humans",
				LicenseExpression: "Apache-2.0",
				License:           "Apache Software License",
				Author:            "Kenneth",
				AuthorEmail:       "kenneth@example.com",
				ProjectURL:        "https://example.com/requests",
			}},
		}},
	}
	got := mapPythonPackageInputs(resp)
	assert.Len(t, got, 1)
	assert.Equal(t, "requests", got[0].Name)
	assert.Equal(t, "", got[0].Group)
	assert.Len(t, got[0].Versions, 1)
	ver := got[0].Versions[0]
	assert.Equal(t, "1.2.3+rhlw.1", ver.Version)
	upstream, release := rhlw.SplitVersion(ver.Version)
	assert.Equal(t, "1.2.3", upstream)
	assert.Equal(t, "rhlw.1", release)
	assert.Equal(t, "pkg:pypi/requests@1.2.3+rhlw.1", ver.Purl)
	assert.Equal(t, "2021-01-01T00:00:00Z", ver.PublishedAt)
	if assert.NotNil(t, ver.Details) {
		assert.Equal(t, "HTTP library", ver.Details.Summary)
		assert.Equal(t, "HTTP library for humans", ver.Details.Description)
		assert.Equal(t, "Apache-2.0", ver.Details.License)
		assert.Equal(t, "Kenneth", ver.Details.Author)
		assert.Equal(t, "kenneth@example.com", ver.Details.AuthorEmail)
		assert.Equal(t, "https://example.com/requests", ver.Details.ProjectURL)
	}
}

func TestMapNpmPackageInputs(t *testing.T) {
	resp := tangy.NpmPackageListResponse{
		Results: []tangy.NpmPackageListItem{{
			Name:           "@types/node",
			Versions:       []string{"20.0.0"},
			LatestVersions: []tangy.NpmVersionInfo{{Version: "20.0.0", CreatedAt: "2022-01-01T00:00:00Z"}},
		}},
	}
	got := mapNpmPackageInputs(resp)
	assert.Len(t, got, 1)
	assert.Equal(t, "node", got[0].Name)
	assert.Equal(t, "@types", got[0].Group)
	assert.Equal(t, "pkg:npm/%40types/node@20.0.0", got[0].Versions[0].Purl)
	assert.Nil(t, got[0].Versions[0].Details)
	_ = config.ContentTypeNpm
}

func TestImportRepo(t *testing.T) {
	tests := []struct {
		name               string
		repo               dao.LightwellRepoToImport
		force              bool
		repoHref           *string
		currentVersion     *string
		expectSyncCalled   bool
		expectUpdateCalled bool
		expectErr          bool
	}{
		{
			name: "unresolved - distribution not found",
			repo: dao.LightwellRepoToImport{
				RepoConfigUUID:              "uuid-1",
				OrgID:                       "org1",
				Name:                        "test-repo",
				ContentType:                 config.ContentTypeMaven,
				BasePath:                    "test/path",
				LastImportRepositoryVersion: "/versions/1/",
			},
			repoHref:           nil, // ResolveRepositoryFromBasePath returns nil
			expectSyncCalled:   false,
			expectUpdateCalled: false,
			expectErr:          false,
		},
		{
			name: "no-change - current version equals stored",
			repo: dao.LightwellRepoToImport{
				RepoConfigUUID:              "uuid-2",
				OrgID:                       "org1",
				Name:                        "test-repo",
				ContentType:                 config.ContentTypeMaven,
				BasePath:                    "test/path",
				LastImportRepositoryVersion: "/versions/5/",
			},
			repoHref:           strPtr("/repo/href/"),
			currentVersion:     strPtr("/versions/5/"), // same as last
			expectSyncCalled:   false,
			expectUpdateCalled: false,
			expectErr:          false,
		},
		{
			name: "change - current version different from stored",
			repo: dao.LightwellRepoToImport{
				RepoConfigUUID:              "uuid-3",
				OrgID:                       "org1",
				Name:                        "test-repo",
				ContentType:                 config.ContentTypeMaven,
				BasePath:                    "test/path",
				LastImportRepositoryVersion: "/versions/5/",
			},
			repoHref:           strPtr("/repo/href/"),
			currentVersion:     strPtr("/versions/6/"), // different from last
			expectSyncCalled:   true,
			expectUpdateCalled: true,
			expectErr:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockPulpClient := pulp_client.NewMockPulpClient(t)
			mockRepoConfigDao := dao.NewMockRepositoryConfigDao(t)
			mockLightwellPkgDao := dao.NewMockLightwellPackageDao(t)
			daoReg := &dao.DaoRegistry{
				RepositoryConfig: mockRepoConfigDao,
				LightwellPackage: mockLightwellPkgDao,
			}

			// Mock ResolveRepositoryFromBasePath
			mockPulpClient.On("ResolveRepositoryFromBasePath", mock.Anything, tt.repo.BasePath).Return(tt.repoHref, nil)

			if tt.repoHref != nil {
				// Mock GetLatestVersionHref
				mockPulpClient.On("GetLatestVersionHref", mock.Anything, *tt.repoHref).Return(tt.currentVersion, nil)

				if tt.expectSyncCalled {
					// Mock ListMavenPackages for the change case
					mockResp := zest.PaginatedMavenRepositoryPackageListResponse{
						Results: []zest.MavenRepositoryPackageResponse{{
							GroupId:    "org.test",
							ArtifactId: "test-artifact",
							Versions:   []string{"1.0"},
							LatestReleases: []zest.MavenPackageReleaseResponse{
								{Version: "1.0", Release: "ga", CreatedAt: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)},
							},
						}},
						Count: 1,
					}
					mockPulpClient.On("ListMavenPackages", mock.Anything, *tt.repoHref, "", importPageSize, 0).Return(mockResp, nil)

					// Mock SyncPackagesForRepository
					mockLightwellPkgDao.On("SyncPackagesForRepository", mock.Anything, tt.repo.RepoConfigUUID, mock.Anything).Return(nil)

					// Mock UpdateLastImportRepositoryVersion
					mockRepoConfigDao.On("InternalOnly_UpdateLastImportRepositoryVersion", mock.Anything, tt.repo.RepoConfigUUID, *tt.currentVersion).Return(nil)
				}
			}

			err := importRepo(context.Background(), daoReg, mockPulpClient, tt.repo, tt.force)

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			mockPulpClient.AssertExpectations(t)
			if tt.expectSyncCalled {
				mockLightwellPkgDao.AssertExpectations(t)
				mockRepoConfigDao.AssertExpectations(t)
			}
		})
	}
}

func strPtr(s string) *string {
	return &s
}
