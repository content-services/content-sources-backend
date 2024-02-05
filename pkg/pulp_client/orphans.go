package pulp_client

import zest "github.com/content-services/zest/release/v2024"

// GetTask Fetch a pulp task
func (r pulpDaoImpl) OrphanCleanup() (string, error) {
	orphansCleanup := *zest.NewOrphansCleanup()
	zero := int64(0)
	orphansCleanup.OrphanProtectionTime = *zest.NewNullableInt64(&zero)
	resp, httpResp, err := r.client.OrphansCleanupAPI.OrphansCleanupCleanup(r.ctx, r.domainName).OrphansCleanup(orphansCleanup).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return "", errorWithResponseBody("error in orphan cleanup", httpResp, err)
	}
	return resp.Task, nil
}
