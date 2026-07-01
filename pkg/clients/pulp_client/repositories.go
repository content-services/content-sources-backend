package pulp_client

import (
	"context"
	"errors"

	zest "github.com/content-services/zest/release/v2026"
)

var errDistributionNotFound = errors.New("repository distribution not found")

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

// Looksup a repository href from a distribution base path in a generic way
func (r *pulpDaoImpl) ResolveRepositoryFromBasePath(ctx context.Context, basePath string) (*string, error) {
	dist, err := r.FindGenericDistributionByBasePath(ctx, basePath)
	if err != nil {
		return nil, err
	}
	if dist == nil {
		return nil, errDistributionNotFound
	}

	repositoryHref := dist.GetRepository()

	// Warning HACK, we are looking up the distribution by base path, and then trying to find the repository from it above,
	//   but some lightwell maven repos use a publication associated with the distribution (no repo link).  However there is no
	//   publication api to pull the publication from. So we must rely on the name of the distribution being the same as the repository,
	//   which for lightwell it will be. Pulp is changing this to not use publications, so this will be temporary, remove after 7/10/2026
	if repositoryHref == "" {
		name := dist.GetName()
		repo, err := r.FindGenericRepositoryByName(ctx, name)
		if err != nil {
			return nil, err
		}
		if repo == nil || repo.PulpHref == nil {
			return nil, nil
		}
		repositoryHref = *repo.PulpHref
	}

	return &repositoryHref, nil
}
