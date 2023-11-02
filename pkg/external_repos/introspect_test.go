package external_repos

//nolint:gci
import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	_ "embed"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/google/uuid"
	"github.com/openlyinc/pointy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestIsRedHatUrl(t *testing.T) {
	assert.True(t, IsRedHat("https://cdn.redhat.com/content/"))
	assert.False(t, IsRedHat("https://someotherdomain.com/myrepo/url"))
}

// https://packages.cloud.google.com/yum/repos/google-compute-engine-el8-x86_64-stable/repodata/repomd.xml
//
//go:embed "test_files/repomd.xml"
var templateRepomdXml []byte

//go:embed "test_files/primary.xml.gz"
var primaryXml []byte

//go:embed "test_files/comps.xml"
var compsXml []byte

const templateRepoMdXmlSum = "f85f0fbfa346372b43e7a9570a76ff6ac57dc26d091f1a6f016e58515c361d33"

func TestIntrospect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/content/repodata/primary.xml.gz":
			{
				var (
					count int
					err   error
				)
				w.Header().Add("Content-Type", "application/gzip")
				body := primaryXml
				if count, err = w.Write(body); err != nil {
					t.Errorf(err.Error())
				}
				if count != len(body) {
					t.Errorf("Not all the body was written")
				}
			}
		case "/content/repodata/repomd.xml":
			{
				var (
					count int
					err   error
				)
				w.Header().Add("Content-Type", "text/xml")
				body := templateRepomdXml
				if count, err = w.Write(body); err != nil {
					t.Errorf(err.Error())
				}
				if count != len(body) {
					t.Errorf("Not all the body was written")
				}
			}
		case "/content/repodata/comps.xml":
			{
				var (
					count int
					err   error
				)
				w.Header().Add("Content-Type", "text/xml")
				body := compsXml
				if count, err = w.Write(body); err != nil {
					t.Errorf(err.Error())
				}
				if count != len(body) {
					t.Errorf("Not all the body was written")
				}
			}
		default:
			{
				var (
					count int
					err   error
				)
				w.Header().Add("Content-Type", "text/plain")
				w.WriteHeader(400)
				content := fmt.Sprintf("Unexpected '%s' path", r.URL.Path)
				body := []byte(content)
				if count, err = w.Write(body); err != nil {
					t.Errorf(err.Error())
				}
				if count != len(body) {
					t.Errorf("Not all the body was written")
				}
				t.Errorf(content)
			}
		}
	}))
	defer server.Close()

	mockDao := dao.GetMockDaoRegistry(t)
	repoUUID := uuid.NewString()
	expected := dao.Repository{
		UUID:           repoUUID,
		URL:            server.URL + "/content",
		RepomdChecksum: templateRepoMdXmlSum,
		PackageCount:   14,
	}
	repoUpdate := RepoToRepoUpdate(expected)
	mockDao.Repository.On("FetchRepositoryRPMCount", repoUUID).Return(14, nil)
	mockDao.Repository.On("Update", repoUpdate).Return(nil).Times(1)
	mockDao.Rpm.On("InsertForRepository", repoUpdate.UUID, mock.Anything).Return(int64(14), nil)
	mockDao.PackageGroup.On("InsertForRepository", repoUpdate.UUID, mock.Anything).Return(int64(1), nil)
	mockDao.Environment.On("InsertForRepository", repoUpdate.UUID, mock.Anything).Return(int64(1), nil)

	count, err, updated := Introspect(
		context.Background(),
		&dao.Repository{
			UUID:         repoUUID,
			URL:          server.URL + "/content",
			PackageCount: 0,
		},
		mockDao.ToDaoRegistry())
	assert.NoError(t, err)
	assert.Equal(t, int64(14), count)
	assert.Equal(t, true, updated)
	assert.Equal(t, 14, expected.PackageCount)

	// Without any changes to the repo, there should be no package updates
	count, err, updated = Introspect(
		context.Background(),
		&dao.Repository{
			UUID:           repoUUID,
			URL:            server.URL + "/content",
			RepomdChecksum: templateRepoMdXmlSum,
			PackageCount:   14,
		},
		mockDao.ToDaoRegistry())
	assert.NoError(t, err)
	assert.Equal(t, int64(0), count)
	assert.Equal(t, false, updated)
	assert.Equal(t, 14, expected.PackageCount)
}

func TestHttpClient(t *testing.T) {
	initialConfig := *config.Get()
	config.LoadedConfig = initialConfig

	client, err := httpClient(false)
	assert.NoError(t, err)
	assert.Equal(t, http.Client{}, client)
}

func TestUpdateIntrospectionStatusMetadata(t *testing.T) {
	// test case 1: status change from pending to valid
	// test case 2: status change from pending to invalid
	// test case 3: status change from valid to unavailable
	// test case 4: status change from unavailable to valid
	// test case 5: status change from invalid to valid
	// test case 6: input is valid and count is 0

	type TestCaseGiven struct {
		status      string
		count       int64
		err         error
		successTime *time.Time
	}

	type TestCase struct {
		name     string
		given    TestCaseGiven
		expected dao.Repository
	}

	timestamp := time.Now()
	testCases := []TestCase{
		{
			name: "Status change from Pending to Valid",
			given: TestCaseGiven{
				status: config.StatusPending,
				count:  1,
				err:    nil,
			},
			expected: dao.Repository{
				LastIntrospectionTime:        &timestamp,
				LastIntrospectionSuccessTime: &timestamp,
				LastIntrospectionUpdateTime:  &timestamp,
				LastIntrospectionError:       pointy.String(""),
				Status:                       config.StatusValid,
				PackageCount:                 100,
			},
		},
		{
			name: "Status change from Pending to Invalid",
			given: TestCaseGiven{
				status: config.StatusPending,
				count:  1,
				err:    fmt.Errorf("Status error: 404"),
			},
			expected: dao.Repository{
				LastIntrospectionTime:  &timestamp,
				LastIntrospectionError: pointy.String("Status error: 404"),
				Status:                 config.StatusInvalid,
				PackageCount:           100,
			},
		},
		{
			name: "Status change from Valid to Unavailable",
			given: TestCaseGiven{
				status: config.StatusValid,
				count:  1,
				err:    fmt.Errorf("Status error: 404"),
			},
			expected: dao.Repository{
				LastIntrospectionTime:  &timestamp,
				LastIntrospectionError: pointy.String("Status error: 404"),
				Status:                 config.StatusUnavailable,
				PackageCount:           100,
			},
		},
		{
			name: "Status change from Unavailable to Valid",
			given: TestCaseGiven{
				status: config.StatusUnavailable,
				count:  1,
				err:    nil,
			},
			expected: dao.Repository{
				LastIntrospectionTime:        &timestamp,
				LastIntrospectionUpdateTime:  &timestamp,
				LastIntrospectionSuccessTime: &timestamp,
				LastIntrospectionError:       pointy.String(""),
				Status:                       config.StatusValid,
				PackageCount:                 100,
			},
		},
		{
			name: "Status change from Invalid to Valid",
			given: TestCaseGiven{
				status: config.StatusInvalid,
				count:  1,
				err:    nil,
			},
			expected: dao.Repository{
				LastIntrospectionTime:        &timestamp,
				LastIntrospectionUpdateTime:  &timestamp,
				LastIntrospectionSuccessTime: &timestamp,
				LastIntrospectionError:       pointy.String(""),
				Status:                       config.StatusValid,
				PackageCount:                 100,
			},
		},
		{
			name: "Status remains as Invalid",
			given: TestCaseGiven{
				status: config.StatusInvalid,
				count:  1,
				err:    fmt.Errorf("Error remains, keep it as Invalid"),
			},
			expected: dao.Repository{
				LastIntrospectionTime:  &timestamp,
				LastIntrospectionError: pointy.String("Error remains, keep it as Invalid"),
				Status:                 config.StatusInvalid,
				PackageCount:           100,
			},
		},
		{
			name: "Status remains as Unavailable",
			given: TestCaseGiven{
				status: config.StatusUnavailable,
				count:  1,
				err:    fmt.Errorf("Error ramins Unavailable"),
			},
			expected: dao.Repository{
				LastIntrospectionTime:  &timestamp,
				LastIntrospectionError: pointy.String("Error ramins Unavailable"),
				Status:                 config.StatusUnavailable,
				PackageCount:           100,
			},
		},
		{
			name: "Status change to Unavailable",
			given: TestCaseGiven{
				status: "AnythingExceptPending",
				count:  1,
				err:    fmt.Errorf("Error set to Unavailable"),
			},
			expected: dao.Repository{
				LastIntrospectionTime:  &timestamp,
				LastIntrospectionError: pointy.String("Error set to Unavailable"),
				Status:                 config.StatusUnavailable,
				PackageCount:           100,
			},
		},
		{
			name: "Status is valid and count is 0",
			given: TestCaseGiven{
				status: config.StatusUnavailable,
				count:  0,
				err:    nil,
			},
			expected: dao.Repository{
				LastIntrospectionTime:        &timestamp,
				LastIntrospectionSuccessTime: &timestamp,
				LastIntrospectionError:       pointy.String(""),
				Status:                       config.StatusValid,
				PackageCount:                 100,
			},
		},
	}

	for _, testCase := range testCases {
		t.Log(testCase.name)

		var givenErr string
		var givenSuccessTime *time.Time
		if testCase.given.err != nil {
			givenErr = testCase.given.err.Error()
		}
		if testCase.given.successTime != nil {
			givenSuccessTime = testCase.given.successTime
		}

		repoIn := dao.Repository{
			LastIntrospectionError:       &givenErr,
			LastIntrospectionSuccessTime: givenSuccessTime,
			Status:                       testCase.given.status,
			PackageCount:                 100,
		}

		result := updateIntrospectionStatusMetadata(
			repoIn,
			testCase.given.count,
			testCase.given.err,
			&timestamp)

		assert.Equal(t, testCase.expected.LastIntrospectionError, result.LastIntrospectionError)
		require.NotNil(t, result.Status)
		assert.Equal(t, testCase.expected.Status, *result.Status)
		assert.Equal(t, testCase.expected.LastIntrospectionTime, result.LastIntrospectionTime)
		assert.Equal(t, testCase.expected.LastIntrospectionSuccessTime, result.LastIntrospectionSuccessTime)
		assert.Equal(t, testCase.expected.LastIntrospectionUpdateTime, result.LastIntrospectionUpdateTime)
	}
}
