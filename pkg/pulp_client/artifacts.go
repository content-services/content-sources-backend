package pulp_client

import (
	"context"
	"fmt"
)

// LookupArtifact checks prescense of an artifact via its checksum
func (r *pulpDaoImpl) LookupArtifact(ctx context.Context, sha256sum string) (*string, error) {
	ctx, client := getZestClient(ctx)

	readResp, httpResp, err := client.ArtifactsAPI.ArtifactsList(ctx, r.domainName).Sha256(sha256sum).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return nil, errorWithResponseBody("creating upload", httpResp, err)
	}
	if len(readResp.Results) == 0 {
		return nil, nil
	} else if len(readResp.Results) == 1 {
		return readResp.Results[0].PulpHref, nil
	} else {
		return readResp.Results[0].PulpHref, fmt.Errorf("Fetched artifact with sha256sum %v, expected at most 1 result, but got %v", sha256sum, len(readResp.Results))
	}
}
