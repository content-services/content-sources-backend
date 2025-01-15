package candlepin_client

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	caliri "github.com/content-services/caliri/release/v4"
	"github.com/content-services/content-sources-backend/pkg/config"
	uuid2 "github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
)

type cpClientImpl struct {
}

const DevelOrgKey = "content-sources-test"
const YumRepoType = "yum"

func errorWithResponseBody(message string, httpResp *http.Response, err error) error {
	if httpResp != nil {
		body, readErr := io.ReadAll(httpResp.Body)
		if readErr != nil {
			log.Logger.Error().Err(readErr).Msg("could not read http body")
		}
		errWithBody := fmt.Errorf("%w: %v", err, string(body))
		return fmt.Errorf("%v: %w", errors.New(message), errWithBody)
	}
	return err
}

func getHTTPClient() (http.Client, error) {
	timeout := 90 * time.Second
	transport := &http.Transport{ResponseHeaderTimeout: timeout}
	var err error

	certStr := config.Get().Clients.Candlepin.ClientCert
	keyStr := config.Get().Clients.Candlepin.ClientKey
	ca := config.Get().Clients.Candlepin.CACert

	if certStr != "" {
		transport, err = config.GetTransport([]byte(certStr), []byte(keyStr), []byte(ca), timeout)
		if err != nil {
			return http.Client{}, fmt.Errorf("could not create http transport: %w", err)
		}
	}
	return http.Client{Transport: transport, Timeout: timeout}, nil
}

func getCorrelationId(ctx context.Context) string {
	value := ctx.Value(config.ContextRequestIDKey{})
	if value != nil {
		valueStr, ok := value.(string)
		if ok {
			return valueStr
		}
	}
	newId := uuid2.NewString()
	log.Logger.Warn().Msgf("Creating correlation ID for candlepin request %v", newId)
	return newId
}

func NewCandlepinClient() CandlepinClient {
	return &cpClientImpl{}
}

func getCandlepinClient(ctx context.Context) (context.Context, *caliri.APIClient, error) {
	httpClient, err := getHTTPClient()
	if err != nil {
		return nil, nil, err
	}

	cpConfig := caliri.NewConfiguration()
	cpConfig.DefaultHeader["X-Correlation-ID"] = getCorrelationId(ctx)
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
	return ctx, client, nil
}
