package handler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

func TestRemoveEndChar(t *testing.T) {
	type TestCaseGiven struct {
		Source string
		Suffix string
	}
	type TestCase struct {
		Name     string
		Given    TestCaseGiven
		Expected string
	}

	testCases := []TestCase{
		{
			Name: "Normal success",
			Given: TestCaseGiven{
				Source: "https://www.example.test/",
				Suffix: "/",
			},
			Expected: "https://www.example.test",
		},
		{
			Name: "Several suffixes",
			Given: TestCaseGiven{
				Source: "https://www.example.test//////",
				Suffix: "/",
			},
			Expected: "https://www.example.test",
		},
		{
			Name: "Empty source string",
			Given: TestCaseGiven{
				Source: "",
				Suffix: "/",
			},
			Expected: "",
		},
		{
			Name: "Empty resulting string",
			Given: TestCaseGiven{
				Source: "//////",
				Suffix: "/",
			},
			Expected: "",
		},
	}

	for _, testCase := range testCases {
		t.Log(testCase.Name)
		var result string
		assert.NotPanics(t, func() {
			result = removeEndSuffix(testCase.Given.Source, testCase.Given.Suffix)
		})
		assert.Equal(t, testCase.Expected, result)
	}
}

type UtilsSuite struct {
	suite.Suite
	reg *dao.MockDaoRegistry
}

func TestUtilsSuite(t *testing.T) {
	suite.Run(t, new(UtilsSuite))
}

func (suite *UtilsSuite) SetupTest() {
	suite.reg = dao.GetMockDaoRegistry(suite.T())
}

func (suite *UtilsSuite) TestFetchSnapshotUUIDsForRepos() {
	t := suite.T()
	orgID := "test-org-123"
	ctx := context.Background()
	testDate := time.Now()

	// Test success path
	urls := []string{"http://example.com/repo1/", "http://example.com/repo2/"}
	repoUUIDs := []string{"uuid-1", "uuid-2"}
	uuids := []string{"uuid-3"}

	suite.reg.RepositoryConfig.On("FetchRepoUUIDsByURLs", ctx, orgID, urls).
		Return(repoUUIDs, nil).Once()

	expectedRequest := api.ListSnapshotByDateRequest{
		RepositoryUUIDS: []string{"uuid-3", "uuid-1", "uuid-2"},
		Date:            testDate,
	}
	snapshotsResp := api.ListSnapshotByDateResponse{
		Data: []api.SnapshotForDate{
			{
				RepositoryUUID: "uuid-1",
				Match: &api.SnapshotResponse{
					UUID: "snapshot-uuid-1",
				},
			},
			{
				RepositoryUUID: "uuid-2",
				Match: &api.SnapshotResponse{
					UUID: "snapshot-uuid-2",
				},
			},
			{
				RepositoryUUID: "uuid-3",
				Match: &api.SnapshotResponse{
					UUID: "snapshot-uuid-3",
				},
			},
		},
	}
	suite.reg.Snapshot.On("FetchSnapshotsByDateAndRepository", ctx, orgID, expectedRequest).
		Return(snapshotsResp, nil).Once()

	result, err := fetchSnapshotUUIDsForRepos(ctx, suite.reg.ToDaoRegistry(), orgID, testDate, urls, uuids)
	assert.NoError(t, err)
	assert.Len(t, result, 3)
	assert.Contains(t, result, "snapshot-uuid-1")
	assert.Contains(t, result, "snapshot-uuid-2")
	assert.Contains(t, result, "snapshot-uuid-3")

	// Test error on repos without snapshots (nil Match)
	urls = []string{"http://example.com/repo1/"}
	uuids = []string{}
	repoUUIDs = []string{"uuid-1"}

	suite.reg.RepositoryConfig.On("FetchRepoUUIDsByURLs", ctx, orgID, urls).
		Return(repoUUIDs, nil).Once()

	expectedRequest = api.ListSnapshotByDateRequest{
		RepositoryUUIDS: []string{"uuid-1"},
		Date:            testDate,
	}
	snapshotsResp = api.ListSnapshotByDateResponse{
		Data: []api.SnapshotForDate{
			{
				RepositoryUUID: "uuid-1",
				Match: &api.SnapshotResponse{
					UUID: "snapshot-uuid-1",
				},
			},
			{
				RepositoryUUID: "uuid-2",
				Match:          nil,
			},
		},
	}
	suite.reg.Snapshot.On("FetchSnapshotsByDateAndRepository", ctx, orgID, expectedRequest).
		Return(snapshotsResp, nil).Once()

	_, err = fetchSnapshotUUIDsForRepos(ctx, suite.reg.ToDaoRegistry(), orgID, testDate, urls, uuids)
	assert.Error(t, err)

	// Test deduplication of UUIDs
	urls = []string{"http://example.com/repo1/"}
	repoUUIDs = []string{"uuid-1"}
	uuids = []string{"uuid-1", "uuid-1", "uuid-2", "uuid-1"}

	suite.reg.RepositoryConfig.On("FetchRepoUUIDsByURLs", ctx, orgID, urls).
		Return(repoUUIDs, nil).Once()

	expectedRequest = api.ListSnapshotByDateRequest{
		RepositoryUUIDS: []string{"uuid-1", "uuid-2"}, // Deduplicated
		Date:            testDate,
	}
	snapshotsResp = api.ListSnapshotByDateResponse{
		Data: []api.SnapshotForDate{
			{
				RepositoryUUID: "uuid-1",
				Match: &api.SnapshotResponse{
					UUID: "snapshot-uuid-1",
				},
			},
			{
				RepositoryUUID: "uuid-2",
				Match: &api.SnapshotResponse{
					UUID: "snapshot-uuid-2",
				},
			},
		},
	}
	suite.reg.Snapshot.On("FetchSnapshotsByDateAndRepository", ctx, orgID, expectedRequest).
		Return(snapshotsResp, nil).Once()

	result, err = fetchSnapshotUUIDsForRepos(ctx, suite.reg.ToDaoRegistry(), orgID, testDate, urls, uuids)
	assert.NoError(t, err)
	assert.Len(t, result, 2)

	// Test error from FetchRepoUUIDsByURLs
	urls = []string{"http://example.com/repo1/"}
	uuids = []string{}

	suite.reg.RepositoryConfig.On("FetchRepoUUIDsByURLs", ctx, orgID, urls).
		Return([]string{}, errors.New("database error")).Once()

	_, err = fetchSnapshotUUIDsForRepos(ctx, suite.reg.ToDaoRegistry(), orgID, testDate, urls, uuids)
	assert.Error(t, err)

	// Test error from FetchSnapshotsByDateAndRepository
	urls = []string{"http://example.com/repo1/"}
	uuids = []string{}
	repoUUIDs = []string{"uuid-1"}

	suite.reg.RepositoryConfig.On("FetchRepoUUIDsByURLs", ctx, orgID, urls).
		Return(repoUUIDs, nil).Once()

	expectedRequest = api.ListSnapshotByDateRequest{
		RepositoryUUIDS: []string{"uuid-1"},
		Date:            testDate,
	}
	suite.reg.Snapshot.On("FetchSnapshotsByDateAndRepository", ctx, orgID, expectedRequest).
		Return(api.ListSnapshotByDateResponse{}, errors.New("snapshots fetch error")).Once()

	_, err = fetchSnapshotUUIDsForRepos(ctx, suite.reg.ToDaoRegistry(), orgID, testDate, urls, uuids)
	assert.Error(t, err)

	// Test with empty inputs
	urls = []string{}
	uuids = []string{}

	suite.reg.RepositoryConfig.On("FetchRepoUUIDsByURLs", ctx, orgID, urls).
		Return([]string{}, nil).Once()

	expectedRequest = api.ListSnapshotByDateRequest{
		RepositoryUUIDS: []string{},
		Date:            testDate,
	}
	snapshotsResp = api.ListSnapshotByDateResponse{
		Data: []api.SnapshotForDate{},
	}
	suite.reg.Snapshot.On("FetchSnapshotsByDateAndRepository", ctx, orgID, expectedRequest).
		Return(snapshotsResp, nil).Once()

	result, err = fetchSnapshotUUIDsForRepos(ctx, suite.reg.ToDaoRegistry(), orgID, testDate, urls, uuids)
	assert.NoError(t, err)
	assert.Empty(t, result)
}
