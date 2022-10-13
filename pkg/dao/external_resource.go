package dao

import (
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/ProtonMail/go-crypto/openpgp"
)

type ExternalResourceDaoImpl struct {
}

func GetExternalResourceDao() ExternalResourceDao {
	return ExternalResourceDaoImpl{}
}

func (erd ExternalResourceDaoImpl) FetchGpgKey(url string) (string, error) {
	timeout := 5 * time.Second
	transport := http.Transport{ResponseHeaderTimeout: timeout}
	client := http.Client{Transport: &transport, Timeout: timeout}

	resp, clientError := client.Get(url)

	if clientError != nil {
		return "", clientError
	}

	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)

	if err != nil {
		return "", err
	}

	gpgKeyString := string(bodyBytes)

	_, openpgpErr := openpgp.ReadArmoredKeyRing(strings.NewReader(gpgKeyString))
	if openpgpErr != nil {
		return "", openpgpErr //Bad key
	}

	return gpgKeyString, err
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

	url, err := UrlToRepomdUrl(url)
	if err != nil {
		return 0, err
	}
	resp, err := client.Head(url)

	if err == nil {
		code = resp.StatusCode
		resp.Body.Close()
	}

	return code, err
}

func UrlToRepomdUrl(urlIn string) (string, error) {
	u, err := url.Parse(urlIn)
	if err != nil {
		return "", err
	}
	u.Path = path.Join(u.Path, "/repodata/repomd.xml")
	return u.String(), nil
}
