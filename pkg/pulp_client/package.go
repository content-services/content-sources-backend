package pulp_client

import (
	"context"
	"fmt"

	zest "github.com/content-services/zest/release/v2024"
)

// specify fields to workaround https://github.com/pulp/pulp_rpm/issues/3694
var RpmFields = []string{"pulp_href", "name", "version", "release", "arch", "epoch", "sha256", "summary"}

func (r *pulpDaoImpl) CreatePackage(ctx context.Context, artifactHref *string, uploadHref *string) (string, error) {
	ctx, client := getZestClient(ctx)

	if artifactHref == nil && uploadHref == nil {
		return "", fmt.Errorf("Must specify either artifactHref or uploadHref")
	}
	if artifactHref != nil && uploadHref != nil {
		return "", fmt.Errorf("Cannot specify both artifactHref and uploadHref")
	}

	api := client.ContentPackagesAPI.ContentRpmPackagesCreate(ctx, r.domainName)
	if artifactHref != nil {
		api = api.Artifact(*artifactHref)
	}
	if uploadHref != nil {
		api = api.Upload(*uploadHref)
	}

	resp, httpResp, err := api.Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return "", errorWithResponseBody("error creating package", httpResp, err)
	}
	return resp.Task, nil
}

func (r *pulpDaoImpl) LookupPackage(ctx context.Context, sha256sum string) (*string, error) {
	ctx, client := getZestClient(ctx)

	resp, httpResp, err := client.ContentPackagesAPI.ContentRpmPackagesList(ctx, r.domainName).Sha256(sha256sum).Fields(RpmFields).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return nil, errorWithResponseBody("error listing packages by sha256sum", httpResp, err)
	}
	if len(resp.Results) == 0 {
		return nil, nil
	} else if len(resp.Results) == 1 {
		return resp.Results[0].PulpHref, nil
	} else {
		return nil, fmt.Errorf("unexpected number of packages listed: %d", len(resp.Results))
	}
}

func (r *pulpDaoImpl) ListVersionPackages(ctx context.Context, versionHref string, offset, limit int32) (pkgs []zest.RpmPackageResponse, total int, err error) {
	ctx, client := getZestClient(ctx)
	resp, httpResp, err := client.ContentPackagesAPI.ContentRpmPackagesList(ctx, r.domainName).RepositoryVersion(versionHref).Limit(limit).Fields(RpmFields).Offset(offset).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return pkgs, 0, errorWithResponseBody("error listing packages for version", httpResp, err)
	}
	return resp.Results, int(resp.Count), err
}

func (r *pulpDaoImpl) ListVersionAllPackages(ctx context.Context, versionHref string) (pkgs []zest.RpmPackageResponse, err error) {
	initial := int32(0)
	limit := int32(300)
	pkgs, total, err := r.ListVersionPackages(ctx, versionHref, initial, limit)
	if err != nil {
		return nil, err
	}
	for len(pkgs) < total {
		initial += limit
		pkgList, _, err := r.ListVersionPackages(ctx, versionHref, initial, limit)
		if err != nil {
			return nil, err
		}
		pkgs = append(pkgs, pkgList...)
	}
	return pkgs, nil
}
