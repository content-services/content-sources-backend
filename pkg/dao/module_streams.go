package dao

import (
	"context"
	"fmt"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/tang/pkg/tangy"
	"gorm.io/gorm"
)

type moduleStreamsImpl struct {
	db *gorm.DB
}

func GetModuleStreamsDao(db *gorm.DB) ModuleStreamsDao {
	// Return DAO instance
	return &moduleStreamsImpl{
		db: db,
	}
}

func (r *moduleStreamsImpl) SearchSnapshotModuleStreams(ctx context.Context, orgID string, request api.SearchSnapshotModuleStreamsRequest) (api.SearchModuleStreamsCollectionResponse, error) {
	if orgID == "" {
		return api.SearchModuleStreamsCollectionResponse{}, fmt.Errorf("orgID can not be an empty string")
	}

	if request.RpmNames == nil {
		request.RpmNames = []string{}
	}

	if request.UUIDs == nil || len(request.UUIDs) == 0 {
		return api.SearchModuleStreamsCollectionResponse{}, &ce.DaoError{
			BadValidation: true,
			Message:       "must contain at least 1 snapshot UUID",
		}
	}

	response := []api.SearchModuleStreams{}

	// Check that snapshot uuids exist
	uuidsValid, uuid := checkForValidSnapshotUuids(ctx, request.UUIDs, r.db)
	if !uuidsValid {
		return api.SearchModuleStreamsCollectionResponse{}, &ce.DaoError{
			NotFound: true,
			Message:  "Could not find snapshot with UUID: " + uuid,
		}
	}

	pulpHrefs := []string{}
	res := readableSnapshots(r.db.WithContext(ctx), orgID).Where("snapshots.UUID in ?", UuidifyStrings(request.UUIDs)).Pluck("version_href", &pulpHrefs)
	if res.Error != nil {
		return api.SearchModuleStreamsCollectionResponse{}, fmt.Errorf("failed to query the db for snapshots: %w", res.Error)
	}
	if config.Tang == nil {
		return api.SearchModuleStreamsCollectionResponse{}, fmt.Errorf("no tang configuration present")
	}

	if len(pulpHrefs) == 0 {
		return api.SearchModuleStreamsCollectionResponse{}, nil
	}

	pkgs, total, err := (*config.Tang).RpmRepositoryVersionModuleStreamsList(ctx, pulpHrefs,
		tangy.ModuleStreamListFilters{RpmNames: request.RpmNames, Search: request.Search}, request.SortBy)

	if err != nil {
		return api.SearchModuleStreamsCollectionResponse{}, fmt.Errorf("error querying module streams in snapshots: %w", err)
	}

	mappedModuleStreams := map[string][]api.Stream{}

	for _, pkg := range pkgs {
		if mappedModuleStreams[pkg.Name] == nil {
			mappedModuleStreams[pkg.Name] = []api.Stream{}
		}
		mappedModuleStreams[pkg.Name] = append(mappedModuleStreams[pkg.Name], api.Stream{
			Name:        pkg.Name,
			Stream:      pkg.Stream,
			Context:     pkg.Context,
			Arch:        pkg.Arch,
			Version:     pkg.Version,
			Description: pkg.Description,
			Profiles:    pkg.Profiles,
		})
	}

	for key, moduleStream := range mappedModuleStreams {
		response = append(response, api.SearchModuleStreams{
			ModuleName: key,
			Streams:    moduleStream,
		})
	}

	return api.SearchModuleStreamsCollectionResponse{
		Data: response,
		Meta: api.ResponseMetadata{
			Count:  int64(total),
			Offset: 0,
			Limit:  5000,
		},
	}, nil
}
