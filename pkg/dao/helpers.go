package dao

import (
	"strings"

	uuid2 "github.com/google/uuid"
)

func UuidifyString(possibleUuid string) uuid2.UUID {
	uuid, err := uuid2.Parse(possibleUuid)
	if err != nil {
		return uuid2.Nil
	}
	return uuid
}

func UuidifyStrings(possibleUuids []string) []uuid2.UUID {
	var uuids []uuid2.UUID
	for _, possibleUuid := range possibleUuids {
		uuids = append(uuids, UuidifyString(possibleUuid))
	}
	return uuids
}

func convertSortByToSQL(SortBy string, SortMap map[string]string, defaultSortBy string) string {
	sqlOrderBy := ""

	sortByArray := strings.Split(SortBy, ",")
	lengthOfSortByParams := len(sortByArray)

	for i := 0; i < lengthOfSortByParams; i++ {
		sortBy := sortByArray[i]

		split := strings.Split(sortBy, ":")
		ascOrDesc := " asc"

		if len(split) > 1 && split[1] == "desc" {
			ascOrDesc = " desc"
		}

		sortField, ok := SortMap[strings.TrimSpace(split[0])]

		// Only update if the SortMap above returns a valid value
		if ok {
			// Concatenate (e.g. "url desc," + "name" + " asc")
			sqlOrderBy = sqlOrderBy + sortField + ascOrDesc

			// Add a comma if this isn't the last item in the "sortByArray".
			if i+1 < lengthOfSortByParams {
				sqlOrderBy = sqlOrderBy + ", "
			}
		}
	}

	if sqlOrderBy == "" && defaultSortBy != "" {
		sqlOrderBy = defaultSortBy
	}

	return sqlOrderBy
}
