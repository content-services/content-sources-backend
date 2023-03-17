package pulp_client

import (
	"context"

	zest "github.com/content-services/zest/release/v3"
)

type pulpDaoImpl struct {
	client *zest.APIClient
	ctx    context.Context
}

func GetPulpClient() PulpClient {
	ctx := context.WithValue(context.Background(), zest.ContextServerIndex, 0)
	pulpConfig := zest.NewConfiguration()

	client := zest.NewAPIClient(pulpConfig)

	auth := context.WithValue(ctx, zest.ContextBasicAuth, zest.BasicAuth{
		UserName: "admin",
		Password: "password",
	})

	// Return DAO instance
	return pulpDaoImpl{
		client: client,
		ctx:    auth,
	}
}
