package pulp_client

import (
	"context"
	"fmt"
	"net/http"

	zest "github.com/content-services/zest/release/v2026"
)

// specify fields to workaround https://github.com/pulp/pulp_rpm/issues/3694
var RpmFields = []string{"pulp_href", "name", "version", "release", "arch", "epoch", "sha256", "summary"}

type versionPackageQueryMode int

const (
	versionPackageQueryPresent versionPackageQueryMode = iota
	versionPackageQueryAdded
	versionPackageQueryRemoved
)

func (r *pulpDaoImpl) CreatePackage(ctx context.Context, artifactHref *string, uploadHref *string) (string, error) {
	ctx, client, err := getZestClient(ctx)
	if err != nil {
		return "", err
	}

	if artifactHref == nil && uploadHref == nil {
		return "", fmt.Errorf("must specify either artifactHref or uploadHref")
	}
	if artifactHref != nil && uploadHref != nil {
		return "", fmt.Errorf("cannot specify both artifactHref and uploadHref")
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
	ctx, client, err := getZestClient(ctx)
	if err != nil {
		return nil, err
	}

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
	ctx, client, err := getZestClient(ctx)
	if err != nil {
		return pkgs, 0, err
	}
	resp, httpResp, err := r.listVersionPackages(ctx, client, versionHref, offset, limit, versionPackageQueryPresent)
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

func (r *pulpDaoImpl) ListVersionAllAddedPackages(ctx context.Context, versionHref string) (pkgs []zest.RpmPackageResponse, err error) {
	return r.listVersionAllPackages(ctx, versionHref, versionPackageQueryAdded)
}

func (r *pulpDaoImpl) ListVersionAllRemovedPackages(ctx context.Context, versionHref string) (pkgs []zest.RpmPackageResponse, err error) {
	return r.listVersionAllPackages(ctx, versionHref, versionPackageQueryRemoved)
}

func (r *pulpDaoImpl) listVersionPackages(
	ctx context.Context,
	client *zest.APIClient,
	versionHref string,
	offset, limit int32,
	mode versionPackageQueryMode,
) (resp *zest.PaginatedrpmPackageResponseList, httpResp *http.Response, err error) {
	req := client.ContentPackagesAPI.ContentRpmPackagesList(ctx, r.domainName).
		Limit(limit).
		Fields(RpmFields).
		Offset(offset)

	switch mode {
	case versionPackageQueryAdded:
		req = req.RepositoryVersionAdded(versionHref)
	case versionPackageQueryRemoved:
		req = req.RepositoryVersionRemoved(versionHref)
	default:
		req = req.RepositoryVersion(versionHref)
	}

	return req.Execute()
}

func (r *pulpDaoImpl) listVersionAllPackages(
	ctx context.Context,
	versionHref string,
	mode versionPackageQueryMode,
) (pkgs []zest.RpmPackageResponse, err error) {
	ctx, client, err := getZestClient(ctx)
	if err != nil {
		return nil, err
	}

	initial := int32(0)
	limit := int32(300)
	resp, httpResp, err := r.listVersionPackages(ctx, client, versionHref, initial, limit, mode)
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return nil, errorWithResponseBody("error listing packages for version", httpResp, err)
	}

	pkgs = resp.Results
	total := int(resp.Count)
	for len(pkgs) < total {
		initial += limit
		pageResp, pageHTTPResp, err := r.listVersionPackages(ctx, client, versionHref, initial, limit, mode)
		if pageHTTPResp != nil {
			defer pageHTTPResp.Body.Close()
		}
		if err != nil {
			return nil, errorWithResponseBody("error listing packages for version", pageHTTPResp, err)
		}
		pkgs = append(pkgs, pageResp.Results...)
	}

	return pkgs, nil
}
