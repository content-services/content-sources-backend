package config

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	ce "github.com/content-services/content-sources-backend/pkg/errors"
)

type CertUser interface {
	ClientCert() string
	ClientKey() string
	CACert() string
	ClientCertPath() string
	ClientKeyPath() string
	CACertPath() string
	Label() string
	Proxy() string
}

func GetHTTPClient(certUser CertUser) (http.Client, error) {
	timeout := 90 * time.Second

	cert, err := ReadCert(certUser)
	if err != nil {
		return http.Client{}, fmt.Errorf("failed to read client certificate: %w", err)
	}

	key, err := ReadKey(certUser)
	if err != nil {
		return http.Client{}, fmt.Errorf("failed to read client key: %w", err)
	}

	caCert, err := ReadCACert(certUser)
	if err != nil {
		return http.Client{}, fmt.Errorf("failed to read client ca certificate: %w", err)
	}

	transport, err := GetTransport(cert, key, caCert, certUser, timeout)
	if err != nil {
		return http.Client{}, fmt.Errorf("error creating http transport: %w", err)
	}

	// retry logic for Pulp clients to handle transient timeout errors
	var roundTripper http.RoundTripper = transport
	if certUser != nil && certUser.Label() == "pulp" {
		retryConfig := DefaultRetryConfig()
		roundTripper = NewRetryTransport(transport, retryConfig)
	}

	return http.Client{Transport: roundTripper, Timeout: timeout}, nil
}

func ReadCert(certUser CertUser) ([]byte, error) {
	var cert []byte
	if certUser.ClientCert() != "" {
		cert = []byte(certUser.ClientCert())
	} else if certUser.ClientCertPath() != "" {
		file, err := os.ReadFile(certUser.ClientCertPath())
		if err != nil {
			return nil, err
		}
		cert = file
	}
	return cert, nil
}

func ReadKey(certUser CertUser) ([]byte, error) {
	var key []byte
	if certUser.ClientKey() != "" {
		key = []byte(certUser.ClientKey())
	} else if certUser.ClientKeyPath() != "" {
		file, err := os.ReadFile(certUser.ClientKeyPath())
		if err != nil {
			return nil, err
		}
		key = file
	}
	return key, nil
}

func ReadCACert(certUser CertUser) ([]byte, error) {
	var caCert []byte
	if certUser.CACert() != "" {
		caCert = []byte(certUser.CACert())
	} else if certUser.CACertPath() != "" {
		file, err := os.ReadFile(certUser.CACertPath())
		if err != nil {
			return nil, err
		}
		caCert = file
	}
	return caCert, nil
}

func GetCertificate(certUser CertUser) (tls.Certificate, error) {
	var tlsCert tls.Certificate

	cert, err := ReadCert(certUser)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to read client certificate: %w", err)
	}

	key, err := ReadKey(certUser)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to read client key: %w", err)
	}

	tlsCert, err = getCertificate(cert, key)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("could not load keypair: %w", err)
	}

	return tlsCert, nil
}

func getCertificate(certBytes, keyBytes []byte) (tls.Certificate, error) {
	var cert tls.Certificate

	if certBytes == nil && keyBytes == nil {
		return tls.Certificate{}, ce.ErrCertKeyNotFound
	}

	cert, err := tls.X509KeyPair(certBytes, keyBytes)
	if err != nil {
		return cert, fmt.Errorf("could not load keypair: %w", err)
	}
	return cert, nil
}

// Shared HTTP transport for all zest clients to utilize connection caching
var sharedTransport http.RoundTripper = &http.Transport{
	DialContext: (&net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}).DialContext,
	MaxIdleConns:          100,
	IdleConnTimeout:       90 * time.Second,
	TLSHandshakeTimeout:   10 * time.Second,
	ExpectContinueTimeout: 1 * time.Second,
	ResponseHeaderTimeout: 60 * time.Second,
}

func GetTransport(certBytes, keyBytes, caCertBytes []byte, certUser CertUser, timeout time.Duration) (*http.Transport, error) {
	var proxyURL *url.URL
	var err error

	transport := &http.Transport{ResponseHeaderTimeout: timeout}
	if certUser != nil && certUser.Proxy() != "" {
		proxyURL, err = url.Parse(certUser.Proxy())
		if err != nil {
			return nil, fmt.Errorf("could not parse proxy URL: %w", err)
		}
		transport.Proxy = http.ProxyURL(proxyURL)
	}

	if certUser != nil && certUser.Label() == "pulp" {
		pulpTransport, ok := sharedTransport.(*http.Transport)
		if !ok {
			return nil, fmt.Errorf("unexpected transport type: %T", sharedTransport)
		}
		pulpTransport.Proxy = transport.Proxy
		transport = pulpTransport
	}

	if certBytes != nil && keyBytes != nil {
		cert, err := getCertificate(certBytes, keyBytes)
		if err != nil {
			return nil, err
		}
		tlsConfig := &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
		}

		if caCertBytes != nil {
			pool, err := certPool(caCertBytes)
			if err != nil {
				return nil, err
			}
			tlsConfig.RootCAs = pool
		}
		transport.TLSClientConfig = tlsConfig
	}
	return transport, nil
}

func certPool(caCert []byte) (*x509.CertPool, error) {
	pool := x509.NewCertPool()
	ok := pool.AppendCertsFromPEM(caCert)
	if !ok {
		return nil, fmt.Errorf("could not parse ca cert")
	}
	return pool, nil
}

func CertUsers() []CertUser {
	var certUsers []CertUser
	if CandlepinConfigured() {
		certUsers = append(certUsers, &CandlepinCertUser{})
	}
	if FeatureServiceConfigured() {
		certUsers = append(certUsers, &FeatureServiceCertUser{})
	}
	if PulpConfigured() {
		certUsers = append(certUsers, &PulpCertUser{})
	}
	return certUsers
}

type FeatureServiceCertUser struct {
}

func (c *FeatureServiceCertUser) ClientCert() string {
	return Get().Clients.FeatureService.ClientCert
}

func (c *FeatureServiceCertUser) ClientKey() string {
	return Get().Clients.FeatureService.ClientKey
}

func (c *FeatureServiceCertUser) CACert() string {
	return Get().Clients.FeatureService.CACert
}

func (c *FeatureServiceCertUser) CACertPath() string {
	return Get().Clients.FeatureService.CACertPath
}

func (c *FeatureServiceCertUser) ClientCertPath() string {
	return Get().Clients.FeatureService.ClientCertPath
}

func (c *FeatureServiceCertUser) ClientKeyPath() string {
	return Get().Clients.FeatureService.ClientKeyPath
}

func (c *FeatureServiceCertUser) Label() string { return "feature_service" }

func (c *FeatureServiceCertUser) Proxy() string {
	return ""
}

type CandlepinCertUser struct {
}

func (c *CandlepinCertUser) ClientCert() string {
	return Get().Clients.Candlepin.ClientCert
}

func (c *CandlepinCertUser) ClientKey() string {
	return Get().Clients.Candlepin.ClientKey
}

func (c *CandlepinCertUser) CACert() string {
	return Get().Clients.Candlepin.CACert
}

func (c *CandlepinCertUser) CACertPath() string {
	return ""
}

func (c *CandlepinCertUser) ClientCertPath() string {
	return ""
}

func (c *CandlepinCertUser) ClientKeyPath() string {
	return ""
}

func (c *CandlepinCertUser) Label() string { return "candlepin" }

func (c *CandlepinCertUser) Proxy() string {
	return ""
}

type PulpCertUser struct{}

func (c *PulpCertUser) ClientCert() string {
	return Get().Clients.Pulp.ClientCert
}

func (c *PulpCertUser) ClientKey() string {
	return Get().Clients.Pulp.ClientKey
}

func (c *PulpCertUser) CACert() string {
	return Get().Clients.Pulp.CACert
}

func (c *PulpCertUser) CACertPath() string {
	return Get().Clients.Pulp.CACertPath
}

func (c *PulpCertUser) ClientCertPath() string {
	return Get().Clients.Pulp.ClientCertPath
}

func (c *PulpCertUser) ClientKeyPath() string {
	return Get().Clients.Pulp.ClientKeyPath
}

func (c *PulpCertUser) Label() string { return "pulp" }

func (c *PulpCertUser) Proxy() string {
	return Get().Clients.Pulp.Proxy
}
