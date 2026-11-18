package commands

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	zest "github.com/content-services/zest/release/v2026"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const (
	testPythonRepoHref = "/api/pulp/lightwell/api/v3/repositories/python/python/0190/"
	testPythonBase     = testPythonRepoHref + "versions/3/"
	testPythonTarget   = testPythonRepoHref + "versions/5/"
)

type pyFile struct {
	name, version, packagetype string
	created                    time.Time
	summary                    string
}

func (f pyFile) row() zest.PythonPythonPackageContentResponse {
	row := zest.PythonPythonPackageContentResponse{}
	row.SetName(f.name)
	row.SetVersion(f.version)
	row.SetPackagetype(f.packagetype)
	row.SetPulpCreated(f.created)
	row.SetSummary(f.summary)
	return row
}

func pyRows(files ...pyFile) []zest.PythonPythonPackageContentResponse {
	rows := make([]zest.PythonPythonPackageContentResponse, 0, len(files))
	for _, f := range files {
		rows = append(rows, f.row())
	}
	return rows
}

func day(d int) time.Time {
	return time.Date(2026, 1, d, 0, 0, 0, 0, time.UTC)
}

// pythonContentStub answers ListPythonPackageContent from fixed file sets. Each page is one file,
// so the reducer sees files split across pages.
type pythonContentStub struct {
	added, removed, current []zest.PythonPythonPackageContentResponse
	addedErr                error
	calls                   []pulp_client.PythonPackageContentFilters
}

func (s *pythonContentStub) list(_ context.Context, f pulp_client.PythonPackageContentFilters, each func([]zest.PythonPythonPackageContentResponse) error) error {
	s.calls = append(s.calls, f)
	var rows []zest.PythonPythonPackageContentResponse
	switch {
	case len(f.AddedBetween) > 0:
		if s.addedErr != nil {
			return s.addedErr
		}
		rows = s.added
	case len(f.RemovedBetween) > 0:
		rows = s.removed
	default:
		names := map[string]bool{}
		for _, n := range f.NameIn {
			names[n] = true
		}
		for _, row := range s.current {
			if len(f.NameIn) == 0 || names[pythonContentKey(row).name] {
				rows = append(rows, row)
			}
		}
	}
	for _, row := range rows {
		if err := each([]zest.PythonPythonPackageContentResponse{row}); err != nil {
			return err
		}
	}
	return nil
}

func (s *pythonContentStub) mock(t *testing.T) *pulp_client.MockPulpClient {
	m := pulp_client.NewMockPulpClient(t)
	m.EXPECT().ListPythonPackageContent(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(s.list).Maybe()
	return m
}

func versionsOf(pkg dao.LightwellPackageInput) []string {
	out := make([]string, 0, len(pkg.Versions))
	for _, v := range pkg.Versions {
		out = append(out, v.Version)
	}
	return out
}

func TestFetchPythonPackagesReducesDistributions(t *testing.T) {
	email := "Jane Doe <jane@example.com>"
	sdist := pyFile{name: "Zope.Interface", version: "6.0", packagetype: "sdist", created: day(1), summary: "from sdist"}.row()
	sdist.SetAuthorEmail(email)
	sdist.SetLicense("ZPL")
	sdist.SetLicenseExpression("")
	sdist.SetDescription("long text")
	sdist.SetProjectUrl("https://example.com/zope")

	stub := &pythonContentStub{current: []zest.PythonPythonPackageContentResponse{
		pyFile{name: "zope_interface", version: "6.0", packagetype: "bdist_wheel", created: day(3), summary: "from wheel"}.row(),
		sdist,
		pyFile{name: "requests", version: "2.0", packagetype: "bdist_wheel", created: day(2), summary: "old wheel"}.row(),
		pyFile{name: "requests", version: "2.0", packagetype: "bdist_wheel", created: day(4), summary: "new wheel"}.row(),
	}}
	got, err := fetchPythonPackages(context.Background(), stub.mock(t), testPythonTarget)
	require.NoError(t, err)

	require.Len(t, stub.calls, 1)
	assert.Equal(t, testPythonTarget, stub.calls[0].RepositoryVersion)
	assert.Equal(t, pythonPackageDetailFields, stub.calls[0].Fields)

	require.Len(t, got, 2)
	assert.Equal(t, "requests", got[0].Name)
	assert.Equal(t, "zope-interface", got[1].Name)
	assert.Equal(t, "", got[1].Group)

	req := got[0].Versions[0]
	assert.Equal(t, "new wheel", req.Details.Summary, "the newest file wins when there is no sdist")
	assert.Equal(t, "2026-01-04T00:00:00Z", req.PublishedAt)

	zope := got[1].Versions[0]
	assert.Equal(t, "6.0", zope.Version)
	assert.Equal(t, "pkg:pypi/zope-interface@6.0", zope.Purl)
	assert.Equal(t, "2026-01-03T00:00:00Z", zope.PublishedAt, "published time is the newest file")
	require.NotNil(t, zope.Details)
	assert.Equal(t, "from sdist", zope.Details.Summary, "the sdist is preferred over a newer wheel")
	assert.Equal(t, "long text", zope.Details.Description)
	assert.Equal(t, "ZPL", zope.Details.License, "license is used when license_expression is empty")
	assert.Equal(t, "Jane Doe", zope.Details.Author, "author falls back to the name in author_email")
	assert.Equal(t, email, zope.Details.AuthorEmail)
	assert.Equal(t, "https://example.com/zope", zope.Details.ProjectURL)
}

func TestFetchPythonPackageChanges(t *testing.T) {
	stub := &pythonContentStub{
		added: pyRows(
			pyFile{name: "requests", version: "2.1", packagetype: "bdist_wheel", created: day(5)},
			pyFile{name: "Django", version: "5.0", packagetype: "bdist_wheel", created: day(5)},
		),
		removed: pyRows(
			pyFile{name: "requests", version: "2.0", packagetype: "sdist"},
			pyFile{name: "requests", version: "2.0", packagetype: "bdist_wheel"},
			pyFile{name: "django", version: "4.2", packagetype: "sdist"},
		),
		current: pyRows(
			pyFile{name: "django", version: "4.1", packagetype: "sdist", created: day(1), summary: "unchanged"},
			pyFile{name: "django", version: "4.2", packagetype: "bdist_wheel", created: day(2), summary: "4.2 wheel"},
			pyFile{name: "django", version: "5.0", packagetype: "sdist", created: day(3), summary: "5.0 sdist"},
			pyFile{name: "django", version: "5.0", packagetype: "bdist_wheel", created: day(5), summary: "5.0 wheel"},
			pyFile{name: "requests", version: "2.1", packagetype: "bdist_wheel", created: day(5), summary: "2.1 wheel"},
			pyFile{name: "unrelated", version: "1.0", packagetype: "sdist", created: day(1)},
		),
	}
	upserts, removals, err := fetchPythonPackageChanges(context.Background(), stub.mock(t), testPythonBase, testPythonTarget)
	require.NoError(t, err)

	require.Len(t, stub.calls, 3)
	assert.Equal(t, []string{testPythonBase, testPythonTarget}, stub.calls[0].AddedBetween)
	assert.Equal(t, pythonPackageKeyFields, stub.calls[0].Fields)
	assert.Equal(t, []string{testPythonBase, testPythonTarget}, stub.calls[1].RemovedBetween)
	assert.Equal(t, testPythonTarget, stub.calls[2].RepositoryVersion)
	assert.Equal(t, []string{"django", "requests"}, stub.calls[2].NameIn, "only packages with changed files are read again")

	require.Len(t, upserts, 2)
	assert.Equal(t, "django", upserts[0].Name)
	assert.Equal(t, []string{"4.2", "5.0"}, versionsOf(upserts[0]), "4.1 did not change")
	assert.Equal(t, "4.2 wheel", upserts[0].Versions[0].Details.Summary, "a version that lost a file is rebuilt from what remains")
	assert.Equal(t, "5.0 sdist", upserts[0].Versions[1].Details.Summary, "a version that gained a file still prefers its sdist")
	assert.Equal(t, "2026-01-05T00:00:00Z", upserts[0].Versions[1].PublishedAt)
	assert.Equal(t, "requests", upserts[1].Name)
	assert.Equal(t, []string{"2.1"}, versionsOf(upserts[1]))

	assert.Equal(t, []dao.LightwellPackageVersionKey{{Group: "", Name: "requests", Version: "2.0"}}, removals)
}

func TestFetchPythonPackageChangesNoChanges(t *testing.T) {
	stub := &pythonContentStub{}
	upserts, removals, err := fetchPythonPackageChanges(context.Background(), stub.mock(t), testPythonBase, testPythonTarget)
	require.NoError(t, err)
	assert.Empty(t, upserts)
	assert.Empty(t, removals)
	assert.Len(t, stub.calls, 2, "nothing changed, so no package is read again")
}

func TestFetchPythonPackageChangesBatchesNames(t *testing.T) {
	stub := &pythonContentStub{}
	for i := 0; i < pythonNameInBatchSize+1; i++ {
		stub.added = append(stub.added, pyFile{name: fmt.Sprintf("pkg-%03d", i), version: "1.0"}.row())
	}
	stub.current = stub.added
	upserts, removals, err := fetchPythonPackageChanges(context.Background(), stub.mock(t), testPythonBase, testPythonTarget)
	require.NoError(t, err)
	require.Len(t, stub.calls, 4)
	assert.Len(t, stub.calls[2].NameIn, pythonNameInBatchSize)
	assert.Len(t, stub.calls[3].NameIn, 1)
	assert.Len(t, upserts, pythonNameInBatchSize+1)
	assert.Empty(t, removals)
}

func TestIncrementalBaseVersion(t *testing.T) {
	base, ok := incrementalBaseVersion(testPythonRepoHref, testPythonBase, false)
	assert.True(t, ok)
	assert.Equal(t, testPythonBase, base)

	_, ok = incrementalBaseVersion(testPythonRepoHref, testPythonBase, true)
	assert.False(t, ok, "forced")
	_, ok = incrementalBaseVersion(testPythonRepoHref, "", false)
	assert.False(t, ok, "first import")
	_, ok = incrementalBaseVersion(testPythonRepoHref, "/api/pulp/lightwell/api/v3/repositories/python/python/other/versions/3/", false)
	assert.False(t, ok, "version of another repository")
}

func TestNeedsTang(t *testing.T) {
	assert.True(t, needsTang(nil))
	assert.True(t, needsTang([]string{config.ContentTypeNpm}))
	assert.False(t, needsTang([]string{config.ContentTypePython}))
	assert.False(t, needsTang([]string{config.ContentTypeMaven}))
}

func TestPythonAuthor(t *testing.T) {
	assert.Equal(t, "Kenneth", pythonAuthor("Kenneth", "Other <o@example.com>"))
	assert.Equal(t, "Jane", pythonAuthor("", "Jane <jane@example.com>"))
	assert.Equal(t, "", pythonAuthor("", "jane@example.com"))
	assert.Equal(t, "", pythonAuthor("", "not an address"))
	assert.Equal(t, "", pythonAuthor("", ""))
}

func TestImportRepoPython(t *testing.T) {
	changes := func() *pythonContentStub {
		return &pythonContentStub{
			added:   pyRows(pyFile{name: "requests", version: "2.1", packagetype: "sdist", created: day(5)}),
			removed: pyRows(pyFile{name: "requests", version: "2.0", packagetype: "sdist"}),
			current: pyRows(
				pyFile{name: "requests", version: "1.0", packagetype: "sdist", created: day(1)},
				pyFile{name: "requests", version: "2.1", packagetype: "sdist", created: day(5)},
			),
		}
	}
	tests := []struct {
		name         string
		stub         *pythonContentStub
		lastVersion  string
		force        bool
		expectFull   bool
		expectedRead int
	}{
		{name: "imported before applies changes", stub: changes(), lastVersion: testPythonBase, expectedRead: 3},
		{name: "first import reads the repository", stub: changes(), lastVersion: "", expectFull: true, expectedRead: 1},
		{name: "forced import reads the repository", stub: changes(), lastVersion: testPythonBase, force: true, expectFull: true, expectedRead: 1},
		{
			name: "failed diff reads the repository",
			stub: func() *pythonContentStub {
				s := changes()
				s.addedErr = fmt.Errorf("base version was deleted")
				return s
			}(),
			lastVersion:  testPythonBase,
			expectFull:   true,
			expectedRead: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := dao.LightwellRepoToImport{
				RepoConfigUUID:              "uuid-python",
				RepositoryUUID:              "repo-python",
				Name:                        "python-repo",
				ContentType:                 config.ContentTypePython,
				BasePath:                    "python/path",
				LastImportRepositoryVersion: tt.lastVersion,
				PackageCount:                1,
				BuildCount:                  2,
				VersionCount:                2,
			}
			pulpClient := tt.stub.mock(t)
			pulpClient.On("ResolveRepositoryFromBasePath", mock.Anything, repo.BasePath).Return(strPtr(testPythonRepoHref), nil)
			pulpClient.On("GetLatestVersionHref", mock.Anything, testPythonRepoHref).Return(strPtr(testPythonTarget), nil)

			pkgDao := dao.NewMockLightwellPackageDao(t)
			repoConfigDao := dao.NewMockRepositoryConfigDao(t)
			daoReg := &dao.DaoRegistry{LightwellPackage: pkgDao, RepositoryConfig: repoConfigDao}

			if tt.expectFull {
				pkgDao.On("SyncPackagesForRepository", mock.Anything, repo.RepoConfigUUID, mock.Anything).
					Run(func(args mock.Arguments) {
						pkgs, ok := args.Get(2).([]dao.LightwellPackageInput)
						require.True(t, ok)
						require.Len(t, pkgs, 1)
						assert.Equal(t, []string{"1.0", "2.1"}, versionsOf(pkgs[0]))
					}).Return(nil).Once()
			} else {
				pkgDao.On("ApplyPackageChangesForRepository", mock.Anything, repo.RepoConfigUUID, mock.Anything, mock.Anything).
					Run(func(args mock.Arguments) {
						upserts, ok := args.Get(2).([]dao.LightwellPackageInput)
						require.True(t, ok)
						removals, ok := args.Get(3).([]dao.LightwellPackageVersionKey)
						require.True(t, ok)
						require.Len(t, upserts, 1)
						assert.Equal(t, []string{"2.1"}, versionsOf(upserts[0]))
						assert.Equal(t, []dao.LightwellPackageVersionKey{{Name: "requests", Version: "2.0"}}, removals)
					}).Return(nil).Once()
			}
			pkgDao.On("InternalOnlyCountStoredContent", mock.Anything, repo.RepoConfigUUID).Return(1, 2, 2, nil).Once()
			repoConfigDao.On("InternalOnly_UpdateLastImportRepositoryVersion", mock.Anything, repo.RepoConfigUUID, testPythonTarget, "").Return(nil).Once()

			require.NoError(t, importRepo(context.Background(), daoReg, pulpClient, repo, tt.force, ""))
			assert.Len(t, tt.stub.calls, tt.expectedRead)
		})
	}
}
