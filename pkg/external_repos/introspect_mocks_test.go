package external_repos

import (
	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/yummy/pkg/yum"
	"github.com/stretchr/testify/mock"
)

// TestIntrospect

type MockRpmDao struct {
	mock.Mock
}

func (m MockRpmDao) List(orgID string, uuidRepo string, limit int, offset int) (api.RepositoryRpmCollectionResponse, int64, error) {
	return api.RepositoryRpmCollectionResponse{}, 0, nil
}

func (m MockRpmDao) InsertForRepository(repoUuid string, pkgs []yum.Package) (int64, error) {
	return 3, nil
}
