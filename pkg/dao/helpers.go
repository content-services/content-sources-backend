package dao

import (
	"fmt"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/api"
	uuid2 "github.com/google/uuid"
	"github.com/openlyinc/pointy"
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

func checkRequestUrlAndUuids(request api.SearchSharedRepositoryEntityRequest) error {
	if len(request.URLs) == 0 && len(request.UUIDs) == 0 {
		return fmt.Errorf("must contain at least 1 URL or 1 UUID")
	}
	return nil
}

func checkRequestLimit(request api.SearchSharedRepositoryEntityRequest) api.SearchSharedRepositoryEntityRequest {
	if request.Limit == nil {
		request.Limit = pointy.Int(api.SearchSharedRepositoryEntityRequestLimitDefault)
	}
	if *request.Limit > api.SearchSharedRepositoryEntityRequestLimitMaximum {
		request.Limit = pointy.Int(api.SearchSharedRepositoryEntityRequestLimitMaximum)
	}
	return request
}

// FIXME 103 Once the URL stored in the database does not
// allow "/" tail characters, this could be removed
func handleTailChars(request api.SearchSharedRepositoryEntityRequest) []string {
	urls := make([]string, len(request.URLs)*2)
	for i, url := range request.URLs {
		urls[i*2] = url
		urls[i*2+1] = url + "/"
	}
	return urls
}
