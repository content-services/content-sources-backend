package pulp_client

import (
	"context"
	"net/http"
	"time"

	"github.com/content-services/content-sources-backend/pkg/config"
	zest "github.com/content-services/zest/release/v2023"
)

const DefaultDomain = "default"

type pulpDaoImpl struct {
	client     *zest.APIClient
	ctx        context.Context
	domainName string
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
	}
	return impl
}
