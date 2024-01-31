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
	"github.com/rs/zerolog/log"
)

type pulpDaoImpl struct {
	client     *zest.APIClient
	ctx        context.Context
	domainName string
	cache      cache.Cache
}

func GetGlobalPulpClient(ctx context.Context) PulpGlobalClient {
	impl := getPulpImpl(ctx)
	return &impl
}

func GetPulpClientWithDomain(ctx context.Context, domainName string) PulpClient {
	impl := getPulpImpl(ctx)
	impl.domainName = domainName
	return &impl
}

func (p *pulpDaoImpl) WithContext(ctx context.Context) PulpClient {
	pulp := getPulpImpl(ctx)
	pulp.domainName = p.domainName
	return &pulp
}

func (p *pulpDaoImpl) WithDomain(domainName string) PulpClient {
	cpy := *p
	cpy.domainName = domainName
	return &cpy
}

func getPulpImpl(ctx context.Context) pulpDaoImpl {
	ctx2 := context.WithValue(ctx, zest.ContextServerIndex, 0)
	timeout := 60 * time.Second
	transport := &http.Transport{ResponseHeaderTimeout: timeout}
	httpClient := http.Client{Transport: transport, Timeout: timeout}

	pulpConfig := zest.NewConfiguration()
	pulpConfig.HTTPClient = &httpClient
	pulpConfig.Servers = zest.ServerConfigurations{zest.ServerConfiguration{
		URL: config.Get().Clients.Pulp.Server,
	}}
	client := zest.NewAPIClient(pulpConfig)

	auth := context.WithValue(ctx2, zest.ContextBasicAuth, zest.BasicAuth{
		UserName: config.Get().Clients.Pulp.Username,
		Password: config.Get().Clients.Pulp.Password,
	})

	impl := pulpDaoImpl{
		client: client,
		ctx:    auth,
		cache:  cache.Initialize(),
	}
	return impl
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
