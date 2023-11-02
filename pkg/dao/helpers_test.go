package dao

import (
	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/openlyinc/pointy"
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

	request := api.SearchSharedRepositoryEntityRequest{
		URLs:   urls,
		UUIDs:  uuids,
		Search: "test",
		Limit:  pointy.Int(1),
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

	request := api.SearchSharedRepositoryEntityRequest{
		URLs:   []string{"http://example.com"},
		UUIDs:  []string{"aaaa-bbbb-cccc"},
		Search: "test",
		Limit:  nil,
	}
	result := checkRequestLimit(request)
	assert.Equal(t, pointy.Int(100), result.Limit)

	request.Limit = pointy.Int(501)
	result = checkRequestLimit(request)
	assert.Equal(t, pointy.Int(500), result.Limit)
}

func (s *RepositorySuite) TestHandleTailChars() {
	t := s.T()

	request := api.SearchSharedRepositoryEntityRequest{
		URLs:   []string{"http://example.com"},
		UUIDs:  []string{"aaaa-bbbb-cccc"},
		Search: "test",
		Limit:  nil,
	}
	result := handleTailChars(request)
	assert.Equal(t, []string{"http://example.com", "http://example.com/"}, result)
}
