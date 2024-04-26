package pulp_client

import (
	"context"

	zest "github.com/content-services/zest/release/v2024"
)

// GetTask Fetch a pulp task
func (r pulpDaoImpl) OrphanCleanup(ctx context.Context) (string, error) {
	ctx, client := getZestClient(ctx)
	orphansCleanup := *zest.NewOrphansCleanup()
	zero := int64(0)
	orphansCleanup.OrphanProtectionTime = *zest.NewNullableInt64(&zero)
	resp, httpResp, err := client.OrphansCleanupAPI.OrphansCleanupCleanup(ctx, r.domainName).OrphansCleanup(orphansCleanup).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return "", errorWithResponseBody("error in orphan cleanup", httpResp, err)
	}
	return resp.Task, nil
}
