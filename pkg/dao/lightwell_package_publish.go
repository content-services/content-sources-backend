package dao

import (
	"context"
	"fmt"

	"github.com/content-services/content-sources-backend/pkg/lightwell/db/store"
)

// PackagePublishTime is the publish timestamp of one mirrored package version.
type PackagePublishTime struct {
	Group       string
	Name        string
	Version     string
	PublishedAt string
}

// PackagePublishTimeLookup reads package publish times from the lightwell catalog.
type PackagePublishTimeLookup struct {
	queries store.Querier
}

func NewPackagePublishTimeLookup(queries store.Querier) PackagePublishTimeLookup {
	return PackagePublishTimeLookup{queries: queries}
}

// List returns publish times for the given package versions. Versions with a
// blank published_at are omitted.
func (l PackagePublishTimeLookup) List(ctx context.Context, versions []string) ([]PackagePublishTime, error) {
	if len(versions) == 0 {
		return nil, nil
	}
	if l.queries == nil {
		return nil, fmt.Errorf("lightwell querier is not initialized")
	}
	rows, err := l.queries.ListPackageVersionPublishTimes(ctx, versions)
	if err != nil {
		return nil, fmt.Errorf("list package publish times: %w", err)
	}
	times := make([]PackagePublishTime, 0, len(rows))
	for _, row := range rows {
		times = append(times, PackagePublishTime{
			Group:       row.PackageGroup,
			Name:        row.Name,
			Version:     row.Version,
			PublishedAt: row.PublishedAt,
		})
	}
	return times, nil
}
