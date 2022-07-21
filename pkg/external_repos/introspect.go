package external_repos

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
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
	RhCdnHost   = "cdn.redhat.com"
	EnvCertPath = "CERT_PATH"
	EnvCaPath   = "CA_PATH"
)

func IntrospectUrl(url string) (int64, error) {
	err, publicRepo := dao.GetPublicRepositoryDao(db.DB).FetchForUrl(url)
	if err != nil {
		return 0, err
	}

	return Introspect(publicRepo)
}

func IsRedHat(url string) bool {
	return strings.Contains(url, RhCdnHost)
}

func Introspect(repo dao.PublicRepository) (int64, error) {
	log.Debug().Msg("Introspecting " + repo.URL)
	client, err := httpClient(IsRedHat(repo.URL))
	if err != nil {
		return 0, err
	}

	pkgs, err := yum.ExtractPackageData(client, repo.URL)

	if err != nil {
		return 0, err
	}
	return dao.GetRpmDao(db.DB).InsertForRepository(repo.UUID, pkgs)
}

func IntrospectAll() (int64, []error) {
	var repos []models.Repository
	var errors []error
	var total int64
	result := db.DB.Find(&repos)
	if result.Error != nil {
		return 0, []error{result.Error}
	}
	for i := 0; i < len(repos); i++ {
		count, err := Introspect(dao.PublicRepository{
			UUID: repos[i].UUID,
			URL:  repos[i].URL,
		})
		total += count
		if err != nil {
			errors = append(errors, err)
		}
	}
	return total, errors
}

func readCertPaths(caPath *string, certPath *string) error {
	const (
		errorCaPathReference   = "caPath cannot be nil"
		errorCertPathReference = "certPath cannot be nil"
		errorCaPathEmpty       = "%s environment var and configuration.certs.ca_path are empty"
		errorCertPathEmpty     = "%s environment var and configuration.certs.cert_path are empty"
	)
	var ok bool
	if caPath == nil {
		return fmt.Errorf(errorCaPathReference)
	}
	if certPath == nil {
		return fmt.Errorf(errorCertPathReference)
	}
	configuration := config.Get()
	if *caPath, ok = os.LookupEnv(EnvCaPath); !ok {
		*caPath = configuration.Certs.CaPath
	}
	if *certPath, ok = os.LookupEnv(EnvCertPath); !ok {
		*certPath = configuration.Certs.CertPath
	}
	if *caPath == "" {
		return fmt.Errorf(errorCaPathEmpty, EnvCaPath)
	}
	if *certPath == "" {
		return fmt.Errorf(errorCertPathEmpty, EnvCertPath)
	}
	return nil
}

func httpClient(useCert bool) (http.Client, error) {
	timeout := 90 * time.Second
	if useCert {
		var (
			filename string
			caFile   string
		)

		if err := readCertPaths(&caFile, &filename); err != nil {
			return http.Client{}, err
		}

		cert, err := tls.LoadX509KeyPair(filename, filename)
		if err != nil {
			return http.Client{}, err
		}

		caCert, err := ioutil.ReadFile(caFile)
		if err != nil {
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
