package external_repos

//nolint:gci
import (
	"fmt"
	"io/ioutil"
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
)

func TestIsRedHatUrl(t *testing.T) {
	assert.True(t, IsRedHat("https://cdn.redhat.com/content/"))
	assert.False(t, IsRedHat("https://someotherdomain.com/myrepo/url"))
}

// https://packages.cloud.google.com/yum/repos/google-compute-engine-el8-x86_64-stable/repodata/repomd.xml
//go:embed "test_files/test_repomd.xml"
var templateRepomdXml []byte

func TestIntrospect(t *testing.T) {
	revisionNumber := "1658448098524979"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/content/repodata/primary.xml.gz":
			{
				var (
					response *http.Response
					err      error
					body     []byte
					count    int
				)
				w.Header().Add("Content-Type", "application/gzip")
				url := "https://packages.cloud.google.com/yum/repos/google-compute-engine-el8-x86_64-stable/repodata/primary.xml.gz"
				if response, err = http.DefaultClient.Get(url); err != nil {
					t.Errorf(err.Error())
				}
				if body, err = ioutil.ReadAll(response.Body); err != nil {
					t.Errorf(err.Error())
				}
				response.Body.Close()
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

	repoUUID := uuid.NewString()
	count, err := Introspect(
		dao.Repository{
			UUID: repoUUID,
			URL:  server.URL + "/content",
		},
		MockRepositoryDao{},
		MockRpmDao{})
	assert.NoError(t, err)
	assert.Equal(t, int64(13), count)

	// Without any changes to the repo, there should be no package updates
	count, err = Introspect(
		dao.Repository{
			UUID:     repoUUID,
			URL:      server.URL + "/content",
			Revision: revisionNumber,
		},
		MockRepositoryDao{},
		MockRpmDao{})
	assert.NoError(t, err)
	assert.Equal(t, int64(0), count)
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
		given    TestCaseGiven
		expected dao.Repository
	}

	timestamp := time.Now()
	testCases := []TestCase{
		// Status change from Pending to Valid
		{
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
			},
		},
		// Status change from Pending to Invalid
		{
			given: TestCaseGiven{
				status: config.StatusPending,
				count:  1,
				err:    fmt.Errorf("Status error: 404"),
			},
			expected: dao.Repository{
				LastIntrospectionTime:  &timestamp,
				LastIntrospectionError: pointy.String("Status error: 404"),
				Status:                 config.StatusInvalid,
			},
		},
		// Status change from Valid to Unavailable
		{
			given: TestCaseGiven{
				status: config.StatusValid,
				count:  1,
				err:    fmt.Errorf("Status error: 404"),
			},
			expected: dao.Repository{
				LastIntrospectionTime:  &timestamp,
				LastIntrospectionError: pointy.String("Status error: 404"),
				Status:                 config.StatusUnavailable,
			},
		},
		// Status change from Unavailable to Valid
		{
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
			},
		},
		// Status change from Invalid to Valid
		{
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
			},
		},
		// Status is valid and count is 0
		{
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
			},
		},
	}

	for i := 0; i < len(testCases); i++ {
		var givenErr string
		if testCases[i].given.err != nil {
			givenErr = testCases[i].given.err.Error()
		}
		givenCount := testCases[i].given.count
		givenStatus := testCases[i].given.status

		expectedErr := testCases[i].expected.LastIntrospectionError
		expectedStatus := testCases[i].expected.Status
		expectedTime := testCases[i].expected.LastIntrospectionTime
		expectedSuccessTime := testCases[i].expected.LastIntrospectionSuccessTime
		expectedUpdateTime := testCases[i].expected.LastIntrospectionUpdateTime

		repoIn := dao.Repository{
			LastIntrospectionError: &givenErr,
			Status:                 givenStatus,
		}
		repoResult := updateIntrospectionStatusMetadata(repoIn, givenCount, testCases[i].given.err, &timestamp)
		assert.Equal(t, expectedStatus, repoResult.Status)
		assert.Equal(t, expectedErr, repoResult.LastIntrospectionError)
		assert.Equal(t, expectedTime, repoResult.LastIntrospectionTime)
		assert.Equal(t, expectedSuccessTime, repoResult.LastIntrospectionSuccessTime)
		assert.Equal(t, expectedUpdateTime, repoResult.LastIntrospectionUpdateTime)
	}
}
