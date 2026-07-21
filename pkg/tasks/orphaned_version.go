package tasks

import (
	"context"

	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/dao"
)

// GetOrphanedLatestVersion returns the latest version href for a repository if it is orphaned, otherwise nil
//
//	If a snapshot is made, but something bad happens between the repo version being made and its publication or distribution,
//		the repo version is 'lost', as there is no snapshot referring to it.  If this happens we can grab the latest
//		repo version from pulp, and check if any snapshot exists with that version href.  If not then this is an orphaned version
func GetOrphanedLatestVersion(ctx context.Context, pulpClient pulp_client.PulpClient, daoReg *dao.DaoRegistry, repoConfigUUID string) (*string, error) {
	repoResp, err := pulpClient.GetRpmRepositoryByName(ctx, repoConfigUUID)
	if err != nil {
		return nil, err
	}
	if repoResp == nil || repoResp.LatestVersionHref == nil {
		return nil, nil
	}
	snap, err := daoReg.Snapshot.FetchSnapshotByVersionHref(ctx, repoConfigUUID, *repoResp.LatestVersionHref)
	if err != nil {
		return nil, err
	}
	// The latest version from pulp is NOT tracked by a snapshot, so return it
	if snap == nil {
		return repoResp.LatestVersionHref, nil
	}
	// It is tracked by a snapshot, so the repo must not have changed
	return nil, nil
}
