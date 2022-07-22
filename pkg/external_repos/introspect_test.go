package external_repos

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsRedHatUrl(t *testing.T) {
	assert.True(t, IsRedHat("https://cdn.redhat.com/content/"))
	assert.False(t, IsRedHat("https://someotherdomain.com/myrepo/url"))
}

func TestIntrospect(t *testing.T) {
	// https://packages.cloud.google.com/yum/repos/google-compute-engine-el8-x86_64-stable/repodata/repomd.xml
	const templateRepomdXml = `<?xml version="1.0" encoding="UTF-8"?>
<repomd xmlns="http://linux.duke.edu/metadata/repo" xmlns:rpm="http://linux.duke.edu/metadata/rpm">
	<revision>1658448098524979</revision>
	<data type="primary">
	<checksum type="sha256">ca42013d44602121b9983217d52571f08a59160982c85277d607954f9cd4c930</checksum>
	<open-checksum type="sha256">c82b2c4b18a77156956327fcae8ac2365ad5191b619c7c1586308e84e7a3561d</open-checksum>
	<location href="repodata/primary.xml.gz"></location>
	<timestamp>1658448098524979</timestamp>
	<size>4271</size>
	<open-size>26550</open-size>
	</data>
	<data type="filelists">
	<checksum type="sha256">a8f4f6c2106508f9eb27b3470421289ff2a6f4c72104c63cca68c42965a710cf</checksum>
	<open-checksum type="sha256">0ad9370b256db5b3f1abfd31b5c02fbc0d2602128235b3140d20fdca2d2b92b0</open-checksum>
	<location href="repodata/filelists.xml.gz"></location>
	<timestamp>1658448098524979</timestamp>
	<size>7386</size>
	<open-size>98292</open-size>
	</data>
	<data type="other">
	<checksum type="sha256">e13969fb0f677c92d7c3268ca5ab8967e3049d9cae1ce252390807de3b8b6e2d</checksum>
	<open-checksum type="sha256">375dabaacebf2707d626fb6c66d8a17214a1532e3d66f6d94e0a94295e075fd6</open-checksum>
	<location href="repodata/other.xml.gz"></location>
	<timestamp>1658448098524979</timestamp>
	<size>1066</size>
	<open-size>2986</open-size>
	</data>
</repomd>`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/content/repodata/primary.xml.gz":
			{
				w.Header().Add("Content-Type", "application/gzip")
				response, _ := http.DefaultClient.Get("https://packages.cloud.google.com/yum/repos/google-compute-engine-el8-x86_64-stable/repodata/primary.xml.gz")
				body, _ := ioutil.ReadAll(response.Body)
				w.Write(body)
			}
		case "/content/repodata/repomd.xml":
			{
				w.Header().Add("Content-Type", "text/xml")
				w.Write([]byte(fmt.Sprintf(templateRepomdXml)))
			}
		default:
			{
				w.Header().Add("Content-Type", "text/plain")
				w.WriteHeader(400)
				content := fmt.Sprintf("Unexpected '%s' path", r.URL.Path)
				w.Write([]byte(content))
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
		MockRpmDao{})
	assert.NoError(t, err)
	assert.Equal(t, int64(3), count)
}

func TestHttpClient(t *testing.T) {
	initialConfig := *config.Get()

	client, err := httpClient(false)
	assert.NoError(t, err)
	assert.Equal(t, http.Client{}, client)

	client, err = httpClient(true)
	assert.NoError(t, err)

	config.LoadedConfig = initialConfig
	config.LoadedConfig.Certs.CaPath = ""
	client, err = httpClient(true)
	require.Error(t, err)
	assert.Equal(t, "Configuration for CA path not found", err.Error())

	config.LoadedConfig = initialConfig
	config.LoadedConfig.Certs.CertPath = ""
	client, err = httpClient(true)
	require.Error(t, err)
	assert.Equal(t, "Configuration for cert path not found", err.Error())
}
