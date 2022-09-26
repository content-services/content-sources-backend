package dao

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUrlToRepomdUrl(t *testing.T) {
	url, err := UrlToRepomdUrl("http://example.com/foo")
	assert.Nil(t, err)
	assert.Equal(t, "http://example.com/foo/repodata/repomd.xml", url)

	url, err = UrlToRepomdUrl("http://example.com/foo/")
	assert.Nil(t, err)
	assert.Equal(t, "http://example.com/foo/repodata/repomd.xml", url)

	_, err = UrlToRepomdUrl("://example.com//")
	assert.Error(t, err)
}

func TestUrlToSigUrl(t *testing.T) {
	url, err := UrlToSigUrl("http://example.com/foo")
	assert.Nil(t, err)
	assert.Equal(t, "http://example.com/foo/repodata/repomd.xml.asc", url)

	url, err = UrlToSigUrl("http://example.com/foo/")
	assert.Nil(t, err)
	assert.Equal(t, "http://example.com/foo/repodata/repomd.xml.asc", url)

	_, err = UrlToSigUrl("://example.com//")
	assert.Error(t, err)
}

func TestFetchRepoMd(t *testing.T) {
	server := externalResourceTestServer(t)
	defer server.Close()

	contents, code, err := GetExternalResourceDao().FetchRepoMd(server.URL + "/content/")
	assert.Equal(t, 200, code)
	assert.NoError(t, err)
	assert.NotNil(t, contents)

	_, code, err = GetExternalResourceDao().FetchRepoMd(server.URL + "/bad_path/")
	assert.Equal(t, 404, code)
	assert.NoError(t, err)
}

func TestFetchSignature(t *testing.T) {
	server := externalResourceTestServer(t)
	defer server.Close()

	contents, code, err := GetExternalResourceDao().FetchSignature(server.URL + "/content/")
	assert.Equal(t, 200, code)
	assert.NoError(t, err)
	assert.NotNil(t, contents)

	_, code, err = GetExternalResourceDao().FetchSignature(server.URL + "/bad_path/")
	assert.Equal(t, 404, code)
	assert.Error(t, err)
}

func externalResourceTestServer(t *testing.T) *httptest.Server {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if (r.URL.Path == "/content/repodata/repomd.xml" || r.URL.Path == "/content/repodata/repomd.xml.asc") && r.Method == "GET" {
			w.WriteHeader(200)
			if _, err := w.Write([]byte{}); err != nil {
				t.Errorf(err.Error())
			}
		} else {
			var (
				count int
				err   error
			)
			w.Header().Add("Content-Type", "text/plain")
			w.WriteHeader(404)
			content := fmt.Sprintf("Unexpected '%s' path", r.URL.Path)
			if count, err = w.Write([]byte(content)); err != nil {
				t.Errorf(err.Error())
			}
			if count != len(content) {
				t.Errorf("Not all the body was written")
			}
		}
	}))
	return server
}

func TestValidGpgKey(t *testing.T) {
	// This is a "PGP public key block Public-Key" which is expected to pass validation.
	gpgKeyString := `-----BEGIN PGP PUBLIC KEY BLOCK-----
Version: GnuPG v1.4.7 (GNU/Linux)

mQENBFYxWIwBCADAKoZhZlJxGNGWzqV+1OG1xiQeoowKhssGAKvd+buXCGISZJwT
LXZqIcIiLP7pqdcZWtE9bSc7yBY2MalDp9Liu0KekywQ6VVX1T72NPf5Ev6x6DLV
7aVWsCzUAF+eb7DC9fPuFLEdxmOEYoPjzrQ7cCnSV4JQxAqhU4T6OjbvRazGl3ag
OeizPXmRljMtUUttHQZnRhtlzkmwIrUivbfFPD+fEoHJ1+uIdfOzZX8/oKHKLe2j
H632kvsNzJFlROVvGLYAk2WRcLu+RjjggixhwiB+Mu/A8Tf4V6b+YppS44q8EvVr
M+QvY7LNSOffSO6Slsy9oisGTdfE39nC7pVRABEBAAG0N01pY3Jvc29mdCAoUmVs
ZWFzZSBzaWduaW5nKSA8Z3Bnc2VjdXJpdHlAbWljcm9zb2Z0LmNvbT6JATUEEwEC
AB8FAlYxWIwCGwMGCwkIBwMCBBUCCAMDFgIBAh4BAheAAAoJEOs+lK2+EinPGpsH
/32vKy29Hg51H9dfFJMx0/a/F+5vKeCeVqimvyTM04C+XENNuSbYZ3eRPHGHFLqe
MNGxsfb7C7ZxEeW7J/vSzRgHxm7ZvESisUYRFq2sgkJ+HFERNrqfci45bdhmrUsy
7SWw9ybxdFOkuQoyKD3tBmiGfONQMlBaOMWdAsic965rvJsd5zYaZZFI1UwTkFXV
KJt3bp3Ngn1vEYXwijGTa+FXz6GLHueJwF0I7ug34DgUkAFvAs8Hacr2DRYxL5RJ
XdNgj4Jd2/g6T9InmWT0hASljur+dJnzNiNCkbn9KbX7J/qK1IbR8y560yRmFsU+
NdCFTW7wY0Fb1fWJ+/KTsC4=
=J6gs
-----END PGP PUBLIC KEY BLOCK-----
`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/fakeGpgKeyUrl" && r.Method == "GET" {
			if _, err := w.Write([]byte(gpgKeyString)); err != nil {
				w.WriteHeader(500)
				t.Errorf(err.Error())
			}
		} else {
			w.WriteHeader(404)
		}
	}))

	defer server.Close()

	str, noErr := GetExternalResourceDao().FetchGpgKey(server.URL + "/fakeGpgKeyUrl")
	assert.Equal(t, gpgKeyString, str)
	assert.NoError(t, noErr)

	emptyStr, err := GetExternalResourceDao().FetchGpgKey(server.URL)
	assert.Equal(t, "", emptyStr)
	assert.Error(t, err)
}

func TestInValidGpgKey(t *testing.T) {
	// This is a "PGP public key block Secret-Key" which is expected to fail validation.
	gpgKeyString := `-----BEGIN PGP PUBLIC KEY BLOCK-----

xsBNBFmUaEEBCACzXTDt6ZnyaVtueZASBzgnAmK13q9Urgch+sKYeIhdymjuMQta
x15OklctmrZtqre5kwPUosG3/B2/ikuPYElcHgGPL4uL5Em6S5C/oozfkYzhwRrT
SQzvYjsE4I34To4UdE9KA97wrQjGoz2Bx72WDLyWwctD3DKQtYeHXswXXtXwKfjQ
7Fy4+Bf5IPh76dA8NJ6UtjjLIDlKqdxLW4atHe6xWFaJ+XdLUtsAroZcXBeWDCPa
buXCDscJcLJRKZVc62gOZXXtPfoHqvUPp3nuLA4YjH9bphbrMWMf810Wxz9JTd3v
yWgGqNY0zbBqeZoGv+TuExlRHT8ASGFS9SVDABEBAAHNNUdpdEh1YiAod2ViLWZs
b3cgY29tbWl0IHNpZ25pbmcpIDxub3JlcGx5QGdpdGh1Yi5jb20+wsBiBBMBCAAW
BQJZlGhBCRBK7hj4Ov3rIwIbAwIZAQAAmQEIACATWFmi2oxlBh3wAsySNCNV4IPf
DDMeh6j80WT7cgoX7V7xqJOxrfrqPEthQ3hgHIm7b5MPQlUr2q+UPL22t/I+ESF6
9b0QWLFSMJbMSk+BXkvSjH9q8jAO0986/pShPV5DU2sMxnx4LfLfHNhTzjXKokws
+8ptJ8uhMNIDXfXuzkZHIxoXk3rNcjDN5c5X+sK8UBRH092BIJWCOfaQt7v7wig5
4Ra28pM9GbHKXVNxmdLpCFyzvyMuCmINYYADsC848QQFFwnd4EQnupo6QvhEVx1O
j7wDwvuH5dCrLuLwtwXaQh0onG4583p0LGms2Mf5F+Ick6o/4peOlBoZz48=
=HXDP
-----END PGP PUBLIC KEY BLOCK-----
`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/fakeGpgKeyUrl" && r.Method == "GET" {
			if _, err := w.Write([]byte(gpgKeyString)); err != nil {
				w.WriteHeader(500)
				t.Errorf(err.Error())
			}
		} else {
			w.WriteHeader(404)
		}
	}))

	defer server.Close()

	str, err := GetExternalResourceDao().FetchGpgKey(server.URL + "/fakeGpgKeyUrl")
	assert.Equal(t, "", str)
	assert.Error(t, err)
}
