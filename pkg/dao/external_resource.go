package dao

import (
	"net/http"
	"time"
)

type ExternalResourceDaoImpl struct {
}

func GetExternalResourceDao() ExternalResourceDao {
	return ExternalResourceDaoImpl{}
}

// ValidRepoMD Does a HEAD request on url/repodata/repomd.xml
//  and returns any error and HTTP code encountered
//  Uses a very short timeout, as this is intended for a
//  small test of validity.  Actual fetching will use a longer timeout
func (erd ExternalResourceDaoImpl) ValidRepoMD(url string) (int, error) {
	var code int
	timeout := 3 * time.Second
	transport := http.Transport{ResponseHeaderTimeout: timeout}
	client := http.Client{Transport: &transport, Timeout: timeout}
	resp, err := client.Head(url)

	if err == nil {
		code = resp.StatusCode
		resp.Body.Close()
	}

	return code, err
}
