package dao

import (
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/utils"
	uuid2 "github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func (s *RepositorySuite) TestConvertSortByToSQL() {
	t := s.T()

	sortMap := map[string]string{
		"name":          "name",
		"url":           "url",
		"package_count": "banana",
		"status":        "status",
	}

	asc := ":asc"
	desc := ":desc"

	result := convertSortByToSQL("name", sortMap, "name asc")
	assert.Equal(t, "name asc", result)

	result = convertSortByToSQL("notInSortMap", sortMap, "name asc")
	assert.Equal(t, "name asc", result)

	result = convertSortByToSQL("url"+asc, sortMap, "name asc")
	assert.Equal(t, "url asc", result)

	result = convertSortByToSQL("package_count", sortMap, "name asc")
	assert.Equal(t, "banana asc", result)

	result = convertSortByToSQL("status"+desc, sortMap, "name asc")
	assert.Equal(t, "status desc", result)

	result = convertSortByToSQL(" status , name:desc", sortMap, "name asc")
	assert.Equal(t, "status asc, name desc", result)
}

func (s *RepositorySuite) TestCheckRequestUrlAndUuids() {
	t := s.T()

	urls := make([]string, 0)
	uuids := make([]string, 0)

	request := api.ContentUnitSearchRequest{
		URLs:   urls,
		UUIDs:  uuids,
		Search: "test",
		Limit:  utils.Ptr(1),
	}
	result := checkRequestUrlAndUuids(request)
	assert.Error(t, result)

	request.UUIDs = []string{"aaaa-bbbb-cccc"}
	result = checkRequestUrlAndUuids(request)
	assert.NoError(t, result)

	request.UUIDs = []string{}
	request.URLs = []string{"http://example.com"}

	result = checkRequestUrlAndUuids(request)
	assert.NoError(t, result)
}

func (s *RepositorySuite) TestCheckRequestLimit() {
	t := s.T()

	request := api.ContentUnitSearchRequest{
		URLs:   []string{"http://example.com"},
		UUIDs:  []string{"aaaa-bbbb-cccc"},
		Search: "test",
		Limit:  nil,
	}
	result := checkRequestLimit(request)
	assert.Equal(t, utils.Ptr(100), result.Limit)

	request.Limit = utils.Ptr(501)
	result = checkRequestLimit(request)
	assert.Equal(t, utils.Ptr(500), result.Limit)
}

func (s *RepositorySuite) TestHandleTailChars() {
	t := s.T()

	request := api.ContentUnitSearchRequest{
		URLs:   []string{"http://example.com"},
		UUIDs:  []string{"aaaa-bbbb-cccc"},
		Search: "test",
		Limit:  nil,
	}
	result := handleTailChars(request)
	assert.Equal(t, []string{"http://example.com", "http://example.com/"}, result)
}

func (s *RepositorySuite) TestUuidifyString() {
	t := s.T()

	init_uuid, err := uuid2.NewUUID()
	assert.NoError(t, err)

	t.Run("valid uuid", func(t *testing.T) {
		result := UuidifyString(init_uuid.String())
		assert.Equal(t, init_uuid, result)
	})

	t.Run("invalid uuid", func(t *testing.T) {
		result := UuidifyString("some-invalid-uuid")
		assert.Equal(t, uuid2.Nil, result)
	})
}

func (s *RepositorySuite) TestCheckForValidSnapshotUuids() {
	t := s.T()
	tx := s.tx

	repo := createRepository(t, tx, "", false)
	snap := createSnapshot(t, tx, repo)
	snap2 := createSnapshot(t, tx, repo)
	invalid_uuid := "some-invalid-uuid"

	t.Run("valid uuid", func(t *testing.T) {
		uuids := []string{snap.UUID}
		result, uuid := checkForValidSnapshotUuids(t.Context(), uuids, s.tx)
		assert.Equal(t, true, result)
		assert.Equal(t, "", uuid)
	})

	t.Run("invalid uuid", func(t *testing.T) {
		uuids := []string{invalid_uuid}
		result, uuid := checkForValidSnapshotUuids(t.Context(), uuids, s.tx)
		assert.Equal(t, false, result)
		assert.Equal(t, invalid_uuid, uuid)
	})

	t.Run("valid uuids", func(t *testing.T) {
		uuids := []string{snap.UUID, snap2.UUID}
		result, uuid := checkForValidSnapshotUuids(t.Context(), uuids, s.tx)
		assert.Equal(t, true, result)
		assert.Equal(t, "", uuid)
	})

	t.Run("valid uuid + invalid uuid", func(t *testing.T) {
		uuids := []string{snap.UUID, invalid_uuid}
		result, uuid := checkForValidSnapshotUuids(t.Context(), uuids, s.tx)
		assert.Equal(t, false, result)
		assert.Equal(t, invalid_uuid, uuid)
	})
}

func (s *RepositorySuite) TestCheckForValidRepoUuidsUrls() {
	t := s.T()
	tx := s.tx

	repo := createRepository(t, tx, "", false)
	invalid_uuid := "some-invalid-uuid"
	invalid_url := "some-invalid-url"

	t.Run("valid uuid and url", func(t *testing.T) {
		uuids := []string{repo.UUID}
		urls := []string{repo.Repository.URL}
		repoValid, urlValid, uuid, url := checkForValidRepoUuidsUrls(t.Context(), uuids, urls, s.tx)
		assert.Equal(t, true, repoValid)
		assert.Equal(t, true, urlValid)
		assert.Equal(t, "", uuid)
		assert.Equal(t, "", url)
	})

	t.Run("invalid uuid and valid url", func(t *testing.T) {
		uuids := []string{invalid_uuid}
		urls := []string{repo.Repository.URL}
		repoValid, urlValid, uuid, url := checkForValidRepoUuidsUrls(t.Context(), uuids, urls, s.tx)
		assert.Equal(t, false, repoValid)
		assert.Equal(t, true, urlValid)
		assert.Equal(t, invalid_uuid, uuid)
		assert.Equal(t, "", url)
	})

	t.Run("valid uuid and invalid url", func(t *testing.T) {
		uuids := []string{repo.UUID}
		urls := []string{invalid_url}
		repoValid, urlValid, uuid, url := checkForValidRepoUuidsUrls(t.Context(), uuids, urls, s.tx)
		assert.Equal(t, true, repoValid)
		assert.Equal(t, false, urlValid)
		assert.Equal(t, "", uuid)
		assert.Equal(t, invalid_url, url)
	})
}
