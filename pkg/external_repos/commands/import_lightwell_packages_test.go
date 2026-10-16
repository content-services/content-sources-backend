package commands

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/handler"
	"github.com/content-services/content-sources-backend/pkg/lightwell/rhlw"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/tang/pkg/tangy"
	zest "github.com/content-services/zest/release/v2026"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestLightwellPackageForceRequest(t *testing.T) {
	original := config.Get().Options.LightwellPackageImportForceAt
	t.Cleanup(func() { config.Get().Options.LightwellPackageImportForceAt = original })

	config.Get().Options.LightwellPackageImportForceAt = ""
	got, err := lightwellPackageForceRequest()
	assert.NoError(t, err)
	assert.Empty(t, got)

	config.Get().Options.LightwellPackageImportForceAt = "  2026-10-09T13:00:00Z  "
	got, err = lightwellPackageForceRequest()
	assert.NoError(t, err)
	assert.Equal(t, "2026-10-09T13:00:00Z", got)

	config.Get().Options.LightwellPackageImportForceAt = "tomorrow"
	_, err = lightwellPackageForceRequest()
	assert.Error(t, err)
}

func TestParseLightwellImportEcosystems(t *testing.T) {
	got, err := parseLightwellImportEcosystems("")
	assert.NoError(t, err)
	assert.Nil(t, got)

	got, err = parseLightwellImportEcosystems(config.ContentTypePython)
	assert.NoError(t, err)
	assert.Equal(t, []string{config.ContentTypePython}, got)

	_, err = parseLightwellImportEcosystems("rpm")
	assert.Error(t, err)
}

func TestShouldImport(t *testing.T) {
	v := "/versions/5/"
	assert.True(t, shouldImport(&v, "/versions/4/", false))   // changed
	assert.False(t, shouldImport(&v, "/versions/5/", false))  // unchanged
	assert.True(t, shouldImport(&v, "/versions/5/", true))    // force
	assert.False(t, shouldImport(nil, "/versions/5/", false)) // nil current -> nothing to import
}

func TestMapMavenFlatPackageInputs(t *testing.T) {
	var calls int
	lookup := newMavenCentralLookup(context.Background(), nil, func(_ context.Context, _, artifactID, _ string) (handler.MavenCentralMetadata, error) {
		calls++
		if artifactID == "lib" {
			return handler.MavenCentralMetadata{Summary: "lib summary", Description: "lib summary"}, nil
		}
		return handler.MavenCentralMetadata{
			Summary:     "commons summary",
			Description: "commons summary",
			License:     "Apache-2.0",
			Author:      "Apache",
			ProjectURL:  "https://example.com/commons",
		}, nil
	})
	rows := []zest.MavenRepositoryFlatPackageResponse{
		{
			GroupId:     "org.apache",
			ArtifactId:  "commons",
			Version:     "1.2.3",
			LastUpdated: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			GroupId:     "org.apache",
			ArtifactId:  "commons",
			Version:     "1.2.3.rhlw-00001",
			LastUpdated: time.Date(2020, 2, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			GroupId:     "org.apache",
			ArtifactId:  "lib",
			Version:     "9.0",
			LastUpdated: time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC),
		},
	}

	got := mapMavenFlatPackageInputs(rows, lookup)
	assert.Len(t, got, 2)
	assert.Equal(t, "commons", got[0].Name)
	assert.Equal(t, "org.apache", got[0].Group)
	assert.Len(t, got[0].Versions, 2)
	assert.Equal(t, "1.2.3", got[0].Versions[0].Version)
	assert.Equal(t, "1.2.3.rhlw-00001", got[0].Versions[1].Version)
	upstream, release := rhlw.SplitVersion(got[0].Versions[1].Version)
	assert.Equal(t, "1.2.3", upstream)
	assert.Equal(t, "rhlw-00001", release)
	assert.Equal(t, "2020-02-01T00:00:00Z", got[0].Versions[1].PublishedAt)
	assert.Equal(t, "pkg:maven/org.apache/commons@1.2.3.rhlw-00001", got[0].Versions[1].Purl)
	assert.Equal(t, "", got[0].Versions[1].Release)
	assert.Same(t, got[0].Versions[0].Details, got[0].Versions[1].Details)
	if assert.NotNil(t, got[0].Versions[0].Details) {
		assert.Equal(t, "commons summary", got[0].Versions[0].Details.Summary)
		assert.Equal(t, "Apache", got[0].Versions[0].Details.Author)
		assert.Equal(t, "", got[0].Versions[0].Details.AuthorEmail)
	}
	assert.Equal(t, "lib", got[1].Name)
	assert.Equal(t, "org.apache", got[1].Group)
	assert.Equal(t, 2, calls)
}

func TestMavenCentralLookup(t *testing.T) {
	flat := zest.MavenRepositoryFlatPackageResponse{
		Description: "from pulp",
		Licenses: []zest.MavenPackageLicenseResponse{
			{Name: "Apache-2.0"},
			{Name: "MIT"},
		},
	}

	t.Run("one fetch per upstream", func(t *testing.T) {
		var calls int
		lookup := newMavenCentralLookup(context.Background(), nil, func(context.Context, string, string, string) (handler.MavenCentralMetadata, error) {
			calls++
			return handler.MavenCentralMetadata{Summary: "from central", ProjectURL: "https://example.com"}, nil
		})
		first := lookup.details("org.apache", "commons", "1.2.3", flat)
		second := lookup.details("org.apache", "commons", "1.2.3", flat)
		assert.Equal(t, 1, calls)
		assert.Same(t, first, second)
	})

	t.Run("maven_packages row skips central", func(t *testing.T) {
		summary := "cached summary"
		license := "Apache-2.0"
		projectURL := "https://example.com/cached"
		author := "Apache"
		mockPackages := dao.NewMockMavenPackagesDao(t)
		mockPackages.On("Fetch", mock.Anything, "org.apache", "commons").Return(&models.MavenPackage{
			GroupID:    "org.apache",
			Name:       "commons",
			Summary:    &summary,
			License:    &license,
			ProjectURL: &projectURL,
			Author:     &author,
		}, nil).Once()
		var calls int
		lookup := newMavenCentralLookup(context.Background(), mockPackages, func(context.Context, string, string, string) (handler.MavenCentralMetadata, error) {
			calls++
			return handler.MavenCentralMetadata{Summary: "from central"}, nil
		})
		first := lookup.details("org.apache", "commons", "1.2.3", flat)
		second := lookup.details("org.apache", "commons", "9.0", flat)
		assert.Equal(t, 0, calls)
		assert.Same(t, first, second)
		if assert.NotNil(t, first) {
			assert.Equal(t, "cached summary", first.Summary)
			assert.Equal(t, "cached summary", first.Description)
			assert.Equal(t, "Apache-2.0", first.License)
			assert.Equal(t, "https://example.com/cached", first.ProjectURL)
			assert.Equal(t, "Apache", first.Author)
			assert.Equal(t, "", first.AuthorEmail)
		}
	})

	t.Run("missing maven package fetches central", func(t *testing.T) {
		mockPackages := dao.NewMockMavenPackagesDao(t)
		mockPackages.On("Fetch", mock.Anything, "org.apache", "commons").Return(nil, nil).Once()
		var calls int
		lookup := newMavenCentralLookup(context.Background(), mockPackages, func(context.Context, string, string, string) (handler.MavenCentralMetadata, error) {
			calls++
			return handler.MavenCentralMetadata{Summary: "from central"}, nil
		})
		first := lookup.details("org.apache", "commons", "1.2.3", flat)
		assert.Equal(t, 1, calls)
		assert.Equal(t, "from central", first.Summary)
	})

	t.Run("maven_packages read error falls back to central", func(t *testing.T) {
		mockPackages := dao.NewMockMavenPackagesDao(t)
		mockPackages.On("Fetch", mock.Anything, "org.apache", "commons").Return(nil, fmt.Errorf("db unavailable")).Once()
		var calls int
		lookup := newMavenCentralLookup(context.Background(), mockPackages, func(context.Context, string, string, string) (handler.MavenCentralMetadata, error) {
			calls++
			return handler.MavenCentralMetadata{Summary: "from central"}, nil
		})
		first := lookup.details("org.apache", "commons", "1.2.3", flat)
		second := lookup.details("org.apache", "commons", "9.0", flat)
		assert.Equal(t, 2, calls)
		assert.Equal(t, "from central", first.Summary)
		assert.Equal(t, "from central", second.Summary)
	})

	t.Run("not found falls back to pulp", func(t *testing.T) {
		lookup := newMavenCentralLookup(context.Background(), nil, func(context.Context, string, string, string) (handler.MavenCentralMetadata, error) {
			return handler.MavenCentralMetadata{}, fmt.Errorf("POM not found: https://example.com/missing.pom")
		})
		got := lookup.details("org.apache", "commons", "1.2.3", flat)
		if assert.NotNil(t, got) {
			assert.Equal(t, "from pulp", got.Summary)
			assert.Equal(t, "from pulp", got.Description)
			assert.Equal(t, "Apache-2.0, MIT", got.License)
			assert.Equal(t, "", got.Author)
			assert.Equal(t, "", got.ProjectURL)
		}
	})

	t.Run("other error writes empty details", func(t *testing.T) {
		lookup := newMavenCentralLookup(context.Background(), nil, func(context.Context, string, string, string) (handler.MavenCentralMetadata, error) {
			return handler.MavenCentralMetadata{}, fmt.Errorf("connection refused")
		})
		got := lookup.details("org.apache", "commons", "1.2.3", flat)
		assert.Equal(t, &dao.LightwellPackageVersionDetailsInput{}, got)
	})
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
		syncErr            bool
		expectErr          bool
		forceRequest       string
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
		{
			name: "force flag imports unchanged version",
			repo: dao.LightwellRepoToImport{
				RepoConfigUUID:              "uuid-4",
				OrgID:                       "org1",
				Name:                        "test-repo",
				ContentType:                 config.ContentTypeMaven,
				BasePath:                    "test/path",
				LastImportRepositoryVersion: "/versions/5/",
			},
			force:              true,
			repoHref:           strPtr("/repo/href/"),
			currentVersion:     strPtr("/versions/5/"),
			expectSyncCalled:   true,
			expectUpdateCalled: true,
			expectErr:          false,
		},
		{
			name: "sync failure does not record the force request",
			repo: dao.LightwellRepoToImport{
				RepoConfigUUID:              "uuid-5",
				OrgID:                       "org1",
				Name:                        "test-repo",
				ContentType:                 config.ContentTypeMaven,
				BasePath:                    "test/path",
				LastImportRepositoryVersion: "/versions/5/",
			},
			repoHref:           strPtr("/repo/href/"),
			currentVersion:     strPtr("/versions/5/"),
			expectSyncCalled:   true,
			expectUpdateCalled: false,
			syncErr:            true,
			expectErr:          true,
			forceRequest:       "2026-10-09T13:00:00Z",
		},
		{
			name: "new force request imports unchanged version",
			repo: dao.LightwellRepoToImport{
				RepoConfigUUID:              "uuid-6",
				OrgID:                       "org1",
				Name:                        "test-repo",
				ContentType:                 config.ContentTypeMaven,
				BasePath:                    "test/path",
				LastImportRepositoryVersion: "/versions/5/",
				PackageImportForcedAt:       "2026-10-09T12:00:00Z",
			},
			repoHref:           strPtr("/repo/href/"),
			currentVersion:     strPtr("/versions/5/"),
			expectSyncCalled:   true,
			expectUpdateCalled: true,
			forceRequest:       "2026-10-09T13:00:00Z",
		},
		{
			name: "completed force request does not import again",
			repo: dao.LightwellRepoToImport{
				RepoConfigUUID:              "uuid-7",
				OrgID:                       "org1",
				Name:                        "test-repo",
				ContentType:                 config.ContentTypeMaven,
				BasePath:                    "test/path",
				LastImportRepositoryVersion: "/versions/5/",
				PackageImportForcedAt:       "2026-10-09T13:00:00Z",
			},
			repoHref:           strPtr("/repo/href/"),
			currentVersion:     strPtr("/versions/5/"),
			expectSyncCalled:   false,
			expectUpdateCalled: false,
			forceRequest:       "2026-10-09T13:00:00Z",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origLoad := loadMavenCentralMetadata
			t.Cleanup(func() { loadMavenCentralMetadata = origLoad })
			loadMavenCentralMetadata = func(context.Context, string, string, string) (handler.MavenCentralMetadata, error) {
				return handler.MavenCentralMetadata{}, nil
			}

			mockPulpClient := pulp_client.NewMockPulpClient(t)
			mockRepoConfigDao := dao.NewMockRepositoryConfigDao(t)
			mockLightwellPkgDao := dao.NewMockLightwellPackageDao(t)
			mockMavenPackages := dao.NewMockMavenPackagesDao(t)
			daoReg := &dao.DaoRegistry{
				RepositoryConfig: mockRepoConfigDao,
				LightwellPackage: mockLightwellPkgDao,
				MavenPackages:    mockMavenPackages,
			}

			// Mock ResolveRepositoryFromBasePath
			mockPulpClient.On("ResolveRepositoryFromBasePath", mock.Anything, tt.repo.BasePath).Return(tt.repoHref, nil)

			if tt.repoHref != nil {
				// Mock GetLatestVersionHref
				mockPulpClient.On("GetLatestVersionHref", mock.Anything, *tt.repoHref).Return(tt.currentVersion, nil)

				if tt.expectSyncCalled {
					mockResp := zest.PaginatedMavenRepositoryFlatPackageResponseList{
						Results: []zest.MavenRepositoryFlatPackageResponse{{
							GroupId:     "org.test",
							ArtifactId:  "test-artifact",
							Version:     "1.0",
							LastUpdated: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
						}},
						Count: 1,
					}
					mockPulpClient.On("ListMavenFlatPackages", mock.Anything, *tt.repoHref).Return(mockResp, nil)
					mockMavenPackages.On("Fetch", mock.Anything, "org.test", "test-artifact").Return(nil, nil)

					syncErr := error(nil)
					if tt.syncErr {
						syncErr = fmt.Errorf("sync failed")
					}
					mockLightwellPkgDao.On("SyncPackagesForRepository", mock.Anything, tt.repo.RepoConfigUUID, mock.Anything).Return(syncErr)

					if tt.expectUpdateCalled {
						mockRepoConfigDao.On("InternalOnly_UpdateLastImportRepositoryVersion", mock.Anything, tt.repo.RepoConfigUUID, *tt.currentVersion, tt.forceRequest).Return(nil)
					}
				}
			}

			err := importRepo(context.Background(), daoReg, mockPulpClient, tt.repo, tt.force, tt.forceRequest)

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
