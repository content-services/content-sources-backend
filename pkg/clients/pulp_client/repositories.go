package pulp_client

import (
	"context"

	zest "github.com/content-services/zest/release/v2026"
)

// FindGenericRepositoryByName finds a repository of any type by name
func (r *pulpDaoImpl) FindGenericRepositoryByName(ctx context.Context, name string) (*zest.RepositoryResponse, error) {
	ctx, client, err := getZestClient(ctx)
	if err != nil {
		return nil, err
	}
	resp, httpResp, err := client.RepositoriesAPI.RepositoriesList(ctx, r.domainName).Name(name).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return nil, errorWithResponseBody("error finding generic repository", httpResp, err)
	}

	results := resp.GetResults()
	if len(results) > 0 {
		return &results[0], nil
	} else {
		return nil, nil
	}
}
