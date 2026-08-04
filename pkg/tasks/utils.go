package tasks

import (
	"context"
	"fmt"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
)

// repositoryDomain returns the Pulp domain that owns a repository's content.
func repositoryDomain(
	ctx context.Context,
	daoReg *dao.DaoRegistry,
	repo api.RepositoryResponse,
	viewerOrgID, viewerDomain, rhDomain, communityDomain string,
) (string, error) {
	switch repo.OrgID {
	case config.RedHatOrg:
		return rhDomain, nil
	case config.CommunityOrg:
		return communityDomain, nil
	case viewerOrgID:
		return viewerDomain, nil
	default:
		ownerDomain, err := daoReg.Domain.Fetch(ctx, repo.OrgID)
		if err != nil {
			return "", fmt.Errorf("error fetching domain for repo org %s: %w", repo.OrgID, err)
		}
		return ownerDomain, nil
	}
}
