package dao

import (
	"strings"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/models"
	uuid2 "github.com/google/uuid"
	"github.com/openlyinc/pointy"
	"gorm.io/gorm"
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

func checkRequestUrlAndUuids(request api.ContentUnitSearchRequest) error {
	if len(request.URLs) == 0 && len(request.UUIDs) == 0 {
		return &ce.DaoError{
			BadValidation: true,
			Message:       "must contain at least 1 URL or 1 UUID",
		}
	}
	return nil
}

func checkRequestLimit(request api.ContentUnitSearchRequest) api.ContentUnitSearchRequest {
	if request.Limit == nil {
		request.Limit = pointy.Int(api.ContentUnitSearchRequestLimitDefault)
	}
	if *request.Limit > api.ContentUnitSearchRequestLimitMaximum {
		request.Limit = pointy.Int(api.ContentUnitSearchRequestLimitMaximum)
	}
	return request
}

// FIXME 103 Once the URL stored in the database does not
// allow "/" tail characters, this could be removed
func handleTailChars(request api.ContentUnitSearchRequest) []string {
	urls := make([]string, len(request.URLs)*2)
	for i, url := range request.URLs {
		urls[i*2] = url
		urls[i*2+1] = url + "/"
	}
	return urls
}

func isOwnedRepository(db *gorm.DB, orgID string, repositoryConfigUUID string) (bool, error) {
	var repoConfigs []models.RepositoryConfiguration
	var count int64
	if err := db.
		Where("org_id IN (?, ?) AND uuid = ?", orgID, config.RedHatOrg, UuidifyString(repositoryConfigUUID)).
		Find(&repoConfigs).
		Count(&count).
		Error; err != nil {
		return false, err
	}
	if count == 0 {
		return false, nil
	}
	return true, nil
}

func checkForValidRepoUuidsUrls(uuids []string, urls []string, db *gorm.DB) (bool, bool, string, string) {
	for _, uuid := range uuids {
		found := models.RepositoryConfiguration{}
		if err := db.
			Where("uuid = ?", uuid).
			First(&found).
			Error; err != nil {
			return false, true, uuid, ""
		}
	}
	for _, url := range urls {
		found := models.Repository{}
		if err := db.
			Where("url = ?", url).
			First(&found).
			Error; err != nil {
			return true, false, "", url
		}
	}
	return true, true, "", ""
}

func checkForValidSnapshotUuids(uuids []string, db *gorm.DB) (bool, string) {
	for _, uuid := range uuids {
		found := models.Snapshot{}
		if err := db.
			Where("uuid = ?", uuid).
			First(&found).
			Error; err != nil {
			return false, uuid
		}
	}
	return true, ""
}
