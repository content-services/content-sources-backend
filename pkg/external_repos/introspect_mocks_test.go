package external_repos

import (
	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/yummy/pkg/yum"
)

// TestIntrospect

type MockRpmDao struct {
}

func (m MockRpmDao) List(orgID string, uuidRepo string, limit int, offset int) (api.RepositoryRpmCollectionResponse, int64, error) {
	return api.RepositoryRpmCollectionResponse{}, 0, nil
}

func (m MockRpmDao) InsertForRepository(repoUuid string, pkgs []yum.Package) (int64, error) {
	return int64(len(pkgs)), nil
}

func (m MockRpmDao) Search(orgID string, request api.SearchRpmRequest, limit int) ([]api.SearchRpmResponse, error) {
	return []api.SearchRpmResponse{}, nil
}
