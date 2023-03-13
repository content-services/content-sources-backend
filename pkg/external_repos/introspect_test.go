package external_repos

//nolint:gci
import (
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

const templateRepoMdXmlSum = "a4e86114143b27e8977b735a354a35cc55100a9e856bcac765cd454dfa4449e2"

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

	mockRepoDao := MockRepositoryDao{}

	repoUUID := uuid.NewString()

	expected := dao.Repository{
		UUID:           repoUUID,
		URL:            server.URL + "/content",
		RepomdChecksum: templateRepoMdXmlSum,
		PackageCount:   14,
	}
	mockRepoDao.On("FetchRepositoryRPMCount", repoUUID).Return(14, nil)
	repoUpdate := RepoToRepoUpdate(expected)

	mockRepoDao.On("Update", repoUpdate).Return(nil).Times(1)

	count, err := Introspect(
		&dao.Repository{
			UUID:         repoUUID,
			URL:          server.URL + "/content",
			PackageCount: 0,
		},
		&mockRepoDao,
		MockRpmDao{})
	assert.NoError(t, err)
	assert.Equal(t, int64(14), count)
	assert.Equal(t, 14, expected.PackageCount)

	// Without any changes to the repo, there should be no package updates
	count, err = Introspect(
		&dao.Repository{
			UUID:           repoUUID,
			URL:            server.URL + "/content",
			RepomdChecksum: templateRepoMdXmlSum,
			PackageCount:   14,
		},
		&mockRepoDao,
		MockRpmDao{})
	assert.NoError(t, err)
	assert.Equal(t, int64(0), count)
	assert.Equal(t, 14, expected.PackageCount)

	mockRepoDao.Mock.AssertExpectations(t)
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
		status string
		count  int64
		err    error
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
		if testCase.given.err != nil {
			givenErr = testCase.given.err.Error()
		}

		repoIn := dao.Repository{
			LastIntrospectionError: &givenErr,
			Status:                 testCase.given.status,
			PackageCount:           100,
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

func TestNeedIntrospect(t *testing.T) {
	type TestCaseExpected struct {
		result bool
		reason string
	}
	type TestCase struct {
		given    *dao.Repository
		expected TestCaseExpected
	}

	var (
		thresholdBefore24 time.Time = time.Now().Add(-(IntrospectTimeInterval - 2*time.Hour)) // Subtract 22 hours to the current time
		thresholdAfter24  time.Time = time.Now().Add(-(IntrospectTimeInterval + time.Hour))   // Subtract 25 hours to the current time
		result            bool
		reason            string
		testCases         []TestCase = []TestCase{
			// When repo is nil
			// it returns false
			{
				given: nil,
				expected: TestCaseExpected{
					result: false,
					reason: "Cannot introspect nil Repository",
				},
			},

			// BEGIN: Cover all the no valid status

			// When Status is not Valid
			// it returns true
			{
				given: &dao.Repository{
					Status: config.StatusInvalid,
				},
				expected: TestCaseExpected{
					result: true,
					reason: fmt.Sprintf("Introspection started: the Status field content differs from '%s' for Repository.UUID = %s", config.StatusValid, ""),
				},
			},
			{
				given: &dao.Repository{
					Status: config.StatusPending,
				},
				expected: TestCaseExpected{
					result: true,
					reason: fmt.Sprintf("Introspection started: the Status field content differs from '%s' for Repository.UUID = %s", config.StatusValid, ""),
				},
			},
			{
				given: &dao.Repository{
					Status: config.StatusUnavailable,
				},
				expected: TestCaseExpected{
					result: true,
					reason: fmt.Sprintf("Introspection started: the Status field content differs from '%s' for Repository.UUID = %s", config.StatusValid, ""),
				},
			},
			// END: Cover all the no valid status

			// When Status is Valid
			// and LastIntrospectionTime is nill
			// it returns true
			{
				given: &dao.Repository{
					Status:                config.StatusValid,
					LastIntrospectionTime: nil,
				},
				expected: TestCaseExpected{
					result: true,
					reason: "Introspection started: not expected LastIntrospectionTime = nil for Repository.UUID = ",
				},
			},
			// When Status is Valid
			// and LastIntrospectionTime does not reach the threshold interval (24hours)
			// it returns false indicating that no introspection is needed
			{
				given: &dao.Repository{
					Status:                config.StatusValid,
					LastIntrospectionTime: &thresholdBefore24,
				},
				expected: TestCaseExpected{
					result: false,
					reason: "Introspection skipped: Last instrospection happened before the threshold for Repository.UUID = ",
				},
			},
			// When Status is Valid
			// and LastIntrospectionTime does reach the threshold interval (24hours)
			// it returns true indicating that an introspection is needed
			{
				given: &dao.Repository{
					Status:                config.StatusValid,
					LastIntrospectionTime: &thresholdAfter24,
				},
				expected: TestCaseExpected{
					result: true,
					reason: "Introspection started: last introspection happened after the threshold for Repository.UUID = ",
				},
			},
		}
	)

	// Run all the test cases
	for _, testCase := range testCases {
		result, reason = needsIntrospect(testCase.given)
		assert.Equal(t, testCase.expected.result, result)
		assert.Equal(t, testCase.expected.reason, reason)
	}
}
