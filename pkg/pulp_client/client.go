package pulp_client

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/content-services/content-sources-backend/pkg/cache"
	"github.com/content-services/content-sources-backend/pkg/config"
	zest "github.com/content-services/zest/release/v2024"
	uuid2 "github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

type pulpDaoImpl struct {
	domainName string
	cache      cache.Cache
}

func GetGlobalPulpClient() PulpGlobalClient {
	impl := getPulpImpl()
	return &impl
}

func GetPulpClientWithDomain(domainName string) PulpClient {
	impl := getPulpImpl()
	impl.domainName = domainName
	return &impl
}

func (p *pulpDaoImpl) WithDomain(domainName string) PulpClient {
	cpy := *p
	cpy.domainName = domainName
	return &cpy
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
	log.Logger.Warn().Msgf("Creating correlation ID for pulp request %v", newId)
	return newId
}

func getZestClient(ctx context.Context) (context.Context, *zest.APIClient) {
	ctx2 := context.WithValue(ctx, zest.ContextServerIndex, 0)
	timeout := 60 * time.Second
	transport := &http.Transport{ResponseHeaderTimeout: timeout}
	httpClient := http.Client{Transport: transport, Timeout: timeout}

	pulpConfig := zest.NewConfiguration()

	pulpConfig.DefaultHeader["Correlation-ID"] = getCorrelationId(ctx)
	pulpConfig.HTTPClient = &httpClient
	pulpConfig.Servers = zest.ServerConfigurations{zest.ServerConfiguration{
		URL: config.Get().Clients.Pulp.Server,
	}}
	client := zest.NewAPIClient(pulpConfig)

	auth := context.WithValue(ctx2, zest.ContextBasicAuth, zest.BasicAuth{
		UserName: config.Get().Clients.Pulp.Username,
		Password: config.Get().Clients.Pulp.Password,
	})

	return auth, client
}

func getPulpImpl() pulpDaoImpl {
	return pulpDaoImpl{
		cache: cache.Initialize(),
	}
}

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
