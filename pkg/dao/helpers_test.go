package dao

import (
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
