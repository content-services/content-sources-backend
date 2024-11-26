package pulp_client

import (
	"context"
	"fmt"

	zest "github.com/content-services/zest/release/v2024"
)

// LookupArtifact checks prescense of an artifact via its checksum
func (r *pulpDaoImpl) LookupArtifact(ctx context.Context, sha256sum string) (*string, error) {
	ctx, client := getZestClient(ctx)

	readResp, httpResp, err := client.ArtifactsAPI.ArtifactsList(ctx, r.domainName).Sha256(sha256sum).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return nil, errorWithResponseBody("error listing artifacts", httpResp, err)
	}
	if len(readResp.Results) == 0 {
		return nil, nil
	} else if len(readResp.Results) == 1 {
		return readResp.Results[0].PulpHref, nil
	} else {
		return readResp.Results[0].PulpHref, fmt.Errorf("fetched artifact with sha256sum %v, expected at most 1 result, but got %v", sha256sum, len(readResp.Results))
	}
}

func (r *pulpDaoImpl) ListArtifacts(ctx context.Context, offset, limit int32) (artifacts []zest.ArtifactResponse, total int, err error) {
	ctx, client := getZestClient(ctx)
	resp, httpResp, err := client.ArtifactsAPI.ArtifactsList(ctx, r.domainName).Fields([]string{"file", "sha256"}).Limit(limit).Offset(offset).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return artifacts, 0, errorWithResponseBody("error listing artifacts", httpResp, err)
	}
	return resp.Results, int(resp.Count), err
}

func (r *pulpDaoImpl) ListAllArtifactSHA256s(ctx context.Context) ([]string, error) {
	initial := int32(0)
	limit := int32(300)

	artifacts, total, err := r.ListArtifacts(ctx, initial, limit)
	if err != nil {
		return nil, err
	}
	for len(artifacts) < total {
		initial += limit
		artifactList, _, err := r.ListArtifacts(ctx, initial, limit)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, artifactList...)
	}

	hashes := make([]string, len(artifacts))
	for i, artifact := range artifacts {
		hashes[i] = *artifact.Sha256.Get()
	}
	return hashes, nil
}
