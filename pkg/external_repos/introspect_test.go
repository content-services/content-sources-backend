package external_repos

//nolint:gci
import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	_ "embed"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/google/uuid"
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
		dao.PublicRepository{
			UUID: repoUUID,
			URL:  server.URL + "/content",
		},
		MockPublicRepositoryDao{},
		MockRpmDao{})
	assert.NoError(t, err)
	assert.Equal(t, int64(13), count)

	// Without any changes to the repo, there should be no package updates
	count, err = Introspect(
		dao.PublicRepository{
			UUID:     repoUUID,
			URL:      server.URL + "/content",
			Revision: revisionNumber,
		},
		MockPublicRepositoryDao{},
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
