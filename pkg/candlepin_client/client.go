package candlepin_client

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"time"

	caliri "github.com/content-services/caliri/release/v4"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/rs/zerolog/log"
)

type cpClientImpl struct {
	client *caliri.APIClient
	ctx    context.Context
}

const DevelOrgKey = "content-sources-test"

func errorWithResponseBody(message string, httpResp *http.Response, err error) error {
	if httpResp != nil {
		body, readErr := io.ReadAll(httpResp.Body)
		if readErr != nil {
			log.Logger.Error().Err(readErr).Msg("could not read http body")
		}
		return fmt.Errorf("%v: %w: %v", message, err, string(body[:]))
	} else {
		return fmt.Errorf("%w: no body", err)
	}
}

func getHTTPClient() (http.Client, error) {
	timeout := 90 * time.Second
	transport := &http.Transport{ResponseHeaderTimeout: timeout}

	certStr := config.Get().Clients.Candlepin.ClientCert
	keyStr := config.Get().Clients.Candlepin.ClientKey

	if certStr != "" {
		cert, err := tls.X509KeyPair([]byte(certStr), []byte(keyStr))
		if err != nil {
			return http.Client{}, fmt.Errorf("could not load cert pair for candlepin %w", err)
		}
		tlsConfig := &tls.Config{
			Certificates: []tls.Certificate{cert},
		}
		transport.TLSClientConfig = tlsConfig
	}
	return http.Client{Transport: transport, Timeout: timeout}, nil
}

func NewCandlepinClient(ctx context.Context) (CandlepinClient, error) {
	httpClient, err := getHTTPClient()
	if err != nil {
		return &cpClientImpl{}, err
	}

	cpConfig := caliri.NewConfiguration()
	cpConfig.HTTPClient = &httpClient
	cpConfig.Servers = caliri.ServerConfigurations{caliri.ServerConfiguration{
		URL: config.Get().Clients.Candlepin.Server,
	}}
	// cpConfig.Debug = true
	client := caliri.NewAPIClient(cpConfig)

	if config.Get().Clients.Candlepin.Username != "" {
		ctx = context.WithValue(ctx, caliri.ContextBasicAuth, caliri.BasicAuth{
			UserName: config.Get().Clients.Candlepin.Username,
			Password: config.Get().Clients.Candlepin.Password,
		})
	}

	impl := cpClientImpl{
		client: client,
		ctx:    ctx,
	}
	return &impl, nil
}
