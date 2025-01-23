package config

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"time"
)

type CertUser interface {
	ClientCert() string
	ClientKey() string
	CACert() string
	ClientCertPath() string
	ClientKeyPath() string
	CACertPath() string
}

func GetHTTPClient(certUser CertUser) (http.Client, error) {
	timeout := 90 * time.Second

	var cert []byte
	if certUser.ClientCert() != "" {
		cert = []byte(certUser.ClientCert())
	} else if certUser.ClientCertPath() != "" {
		file, err := os.ReadFile(certUser.ClientCertPath())
		if err != nil {
			return http.Client{}, err
		}
		cert = file
	}

	var key []byte
	if certUser.ClientKey() != "" {
		key = []byte(certUser.ClientKey())
	} else if certUser.ClientKeyPath() != "" {
		file, err := os.ReadFile(certUser.ClientKeyPath())
		if err != nil {
			return http.Client{}, err
		}
		key = file
	}

	var caCert []byte
	if certUser.CACert() != "" {
		caCert = []byte(certUser.CACert())
	} else if certUser.CACertPath() != "" {
		file, err := os.ReadFile(certUser.CACertPath())
		if err != nil {
			return http.Client{}, err
		}
		caCert = file
	}

	transport, err := GetTransport(cert, key, caCert, timeout)
	if err != nil {
		return http.Client{}, fmt.Errorf("error creating http transport: %w", err)
	}

	return http.Client{Transport: transport, Timeout: timeout}, nil
}

func GetTransport(certBytes, keyBytes, caCertBytes []byte, timeout time.Duration) (*http.Transport, error) {
	transport := &http.Transport{ResponseHeaderTimeout: timeout}

	if certBytes != nil && keyBytes != nil {
		cert, err := tls.X509KeyPair(certBytes, keyBytes)
		if err != nil {
			return transport, fmt.Errorf("could not load keypair: %w", err)
		}
		tlsConfig := &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
		}

		if caCertBytes != nil {
			pool, err := certPool(caCertBytes)
			if err != nil {
				return transport, err
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
