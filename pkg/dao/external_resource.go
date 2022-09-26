package dao

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/ProtonMail/go-crypto/openpgp"
)

const RequestTimeout = 3 * time.Second

type ExternalResourceDaoImpl struct {
}

func GetExternalResourceDao() ExternalResourceDao {
	return ExternalResourceDaoImpl{}
}

func (erd ExternalResourceDaoImpl) FetchGpgKey(url string) (string, error) {
	gpgKeyString, code, err := erd.fetchFile(url)
	if err == nil && code < 200 || code > 299 {
		err = &Error{Message: fmt.Sprintf("Received HTTP %d", code)}
	}

	_, openpgpErr := openpgp.ReadArmoredKeyRing(strings.NewReader(*gpgKeyString))
	if openpgpErr != nil {
		return "", openpgpErr //Bad key
	}

	return *gpgKeyString, err
}

func (erd ExternalResourceDaoImpl) fetchFile(url string) (*string, int, error) {
	var sigBody string
	var code int

	transport := http.Transport{ResponseHeaderTimeout: RequestTimeout}
	client := http.Client{Transport: &transport, Timeout: RequestTimeout}

	resp, err := client.Get(url)

	if err == nil {
		bytes, err := ioutil.ReadAll(resp.Body)
		sigBody = string(bytes)
		code = resp.StatusCode
		resp.Body.Close()
		return &sigBody, code, err
	} else {
		return nil, code, err
	}
}

// FetchSignature fetches the yum metadata signature
//  and returns any error and HTTP code encountered along with the contents.
//  Uses a very short timeout, as this is intended for a
//  small test of validity.  Actual fetching will use a longer timeout
func (erd ExternalResourceDaoImpl) FetchSignature(repoUrl string) (*string, int, error) {
	sigUrl, err := UrlToSigUrl(repoUrl)
	if err != nil {
		return nil, 0, err
	}
	sig, code, err := erd.fetchFile(sigUrl)
	if err == nil && code < 200 || code > 299 {
		err = &Error{Message: fmt.Sprintf("Received HTTP %d", code)}
	}
	return sig, code, err
}

// FetchRepoMd Does a Get request on url/repodata/repomd.xml
//  and returns any error and HTTP code encountered along with the contents.
//  Uses a very short timeout, as this is intended for a
//  small test of validity.  Actual fetching will use a longer timeout
func (erd ExternalResourceDaoImpl) FetchRepoMd(repoUrl string) (*string, int, error) {
	sigUrl, err := UrlToRepomdUrl(repoUrl)
	if err != nil {
		return nil, 0, err
	}
	return erd.fetchFile(sigUrl)
}

func UrlToRepomdUrl(urlIn string) (string, error) {
	u, err := url.Parse(urlIn)
	if err != nil {
		return "", err
	}
	u.Path = path.Join(u.Path, "/repodata/repomd.xml")
	return u.String(), nil
}

func UrlToSigUrl(urlIn string) (string, error) {
	url, err := UrlToRepomdUrl(urlIn)
	if err == nil {
		return url + ".asc", nil
	} else {
		return "", err
	}
}
