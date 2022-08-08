package dao

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type mockExternalResource struct {
	mock.Mock
}

func (erd *mockExternalResource) ValidRepoMD(url string) (int, error) {
	args := erd.Called(url)
	return args.Int(0), args.Error(1)
}

func TestValidRepoMD(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/content/repodata/repomd.xml" && r.Method == "HEAD" {
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
	defer server.Close()

	code, err := GetExternalResourceDao().ValidRepoMD(server.URL + "/content/repodata/repomd.xml")
	assert.Equal(t, code, 200)
	assert.NoError(t, err)

	code, err = GetExternalResourceDao().ValidRepoMD(server.URL + "/bad_path/repodata/repomd.xml")
	assert.Equal(t, code, 404)
	assert.NoError(t, err)
}
