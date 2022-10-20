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

	result := convertSortByToSQL("name", sortMap)
	assert.Equal(t, "name asc", result)

	result = convertSortByToSQL("notInSortMap", sortMap)
	assert.Equal(t, "name asc", result)

	result = convertSortByToSQL("url"+asc, sortMap)
	assert.Equal(t, "url asc", result)

	result = convertSortByToSQL("package_count", sortMap)
	assert.Equal(t, "banana asc", result)

	result = convertSortByToSQL("status"+desc, sortMap)
	assert.Equal(t, "status desc", result)

	result = convertSortByToSQL(" status , name:desc", sortMap)
	assert.Equal(t, "status asc, name desc", result)
}
