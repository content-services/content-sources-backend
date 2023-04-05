package pulp_client

import (
	"context"

	"github.com/content-services/content-sources-backend/pkg/config"
	zest "github.com/content-services/zest/release/v3"
)

type pulpDaoImpl struct {
	client *zest.APIClient
	ctx    context.Context
}

func GetPulpClient() PulpClient {
	ctx := context.WithValue(context.Background(), zest.ContextServerIndex, 0)
	pulpConfig := zest.NewConfiguration()
	pulpConfig.Servers = zest.ServerConfigurations{zest.ServerConfiguration{
		URL: config.Get().Clients.Pulp.Server,
	}}
	client := zest.NewAPIClient(pulpConfig)

	auth := context.WithValue(ctx, zest.ContextBasicAuth, zest.BasicAuth{
		UserName: config.Get().Clients.Pulp.Username,
		Password: config.Get().Clients.Pulp.Password,
	})

	// Return DAO instance
	return pulpDaoImpl{
		client: client,
		ctx:    auth,
	}
}
