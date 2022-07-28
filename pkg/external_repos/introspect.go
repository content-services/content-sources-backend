package external_repos

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/yummy/pkg/yum"
	"github.com/rs/zerolog/log"
)

const (
	RhCdnHost = "cdn.redhat.com"
)

// IntrospectUrl Fetch the metadata of a url and insert RPM data
//  Returns the number of new RPMs inserted system-wide and any error encountered
func IntrospectUrl(url string) (int64, error) {
	err, publicRepo := dao.GetPublicRepositoryDao(db.DB).FetchForUrl(url)
	rpmDao := dao.GetRpmDao(db.DB, nil)
	if err != nil {
		return 0, err
	}

	return Introspect(publicRepo, rpmDao)
}

// IsRedHat returns if the url is a 'cdn.redhat.com' url
func IsRedHat(url string) bool {
	return strings.Contains(url, RhCdnHost)
}

// Introspect introspects a dao.PublicRepository with the given RpmDao
// 	inserting any needed RPMs and adding and removing associations to the repository
//  Returns the number of new RPMs inserted system-wide and any error encountered
func Introspect(repo dao.PublicRepository, rpm dao.RpmDao) (int64, error) {
	var (
		client http.Client
		err    error
		pkgs   []yum.Package
	)
	log.Debug().Msg("Introspecting " + repo.URL)

	if client, err = httpClient(IsRedHat(repo.URL)); err != nil {
		return 0, err
	}
	if pkgs, err = yum.ExtractPackageData(client, repo.URL); err != nil {
		return 0, err
	}
	return rpm.InsertForRepository(repo.UUID, pkgs)
}

// IntrospectAll introspects all repositories
//  Returns the number of new RPMs inserted system-wide and all errors encountered
func IntrospectAll() (int64, []error) {
	var repos []models.Repository
	var errors []error
	var total int64
	var count int64
	var err error
	thisdb := db.DB
	rpmDao := dao.GetRpmDao(thisdb, nil)
	result := thisdb.Find(&repos)
	if result.Error != nil {
		return 0, []error{result.Error}
	}
	for i := 0; i < len(repos); i++ {
		publicRepo := dao.PublicRepository{
			UUID: repos[i].UUID,
			URL:  repos[i].URL,
		}
		count, err = Introspect(publicRepo, rpmDao)
		total += count
		if err != nil {
			errors = append(errors, err)
		}
	}
	return total, errors
}

func httpClient(useCert bool) (http.Client, error) {
	timeout := 90 * time.Second
	if useCert {
		var (
			cert   tls.Certificate
			caCert []byte
			err    error
		)
		configuration := config.Get()

		if configuration.Certs.CertPath == "" {
			return http.Client{}, fmt.Errorf("Configuration for cert path not found")
		}

		cert, err = tls.LoadX509KeyPair(configuration.Certs.CertPath, configuration.Certs.CertPath)
		if err != nil {
			return http.Client{}, err
		}

		if caCert, err = LoadCA(); err != nil {
			return http.Client{}, err
		}
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(caCert)

		tlsConfig := &tls.Config{
			Certificates: []tls.Certificate{cert},
			RootCAs:      caCertPool,
		}

		transport := &http.Transport{TLSClientConfig: tlsConfig, ResponseHeaderTimeout: timeout}
		return http.Client{Transport: transport, Timeout: timeout}, nil
	} else {
		return http.Client{}, nil
	}
}
