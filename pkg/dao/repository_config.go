package dao

import (
	"fmt"
	"strings"

	openpgp "github.com/ProtonMail/go-crypto/openpgp"
	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/jackc/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type repositoryConfigDaoImpl struct {
	db        *gorm.DB
	extResDao ExternalResourceDao
}

func GetRepositoryConfigDao(db *gorm.DB) RepositoryConfigDao {
	return repositoryConfigDaoImpl{
		db:        db,
		extResDao: GetExternalResourceDao(),
	}
}

func DBErrorToApi(e error) *Error {
	var dupKeyName string
	pgError, ok := e.(*pgconn.PgError)
	if ok {
		if pgError.Code == "23505" {
			switch pgError.ConstraintName {
			case "repo_and_org_id_unique":
				dupKeyName = "URL"
			case "name_and_org_id_unique":
				dupKeyName = "name"
			}
			return &Error{BadValidation: true, Message: "Repository with this " + dupKeyName + " already belongs to organization"}
		}
	}
	dbError, ok := e.(models.Error)
	if ok {
		return &Error{BadValidation: dbError.Validation, Message: dbError.Message}
	}
	return &Error{Message: e.Error()}
}

func (r repositoryConfigDaoImpl) Create(newRepoReq api.RepositoryRequest) (api.RepositoryResponse, error) {
	var newRepo models.Repository
	var newRepoConfig models.RepositoryConfiguration
	ApiFieldsToModel(newRepoReq, &newRepoConfig, &newRepo)

	if err := r.db.Where("url = ?", newRepo.URL).FirstOrCreate(&newRepo).Error; err != nil {
		return api.RepositoryResponse{}, DBErrorToApi(err)
	}

	if newRepoReq.OrgID != nil {
		newRepoConfig.OrgID = *newRepoReq.OrgID
	}
	if newRepoReq.AccountID != nil {
		newRepoConfig.AccountID = *newRepoReq.AccountID
	}
	newRepoConfig.RepositoryUUID = newRepo.Base.UUID

	if err := r.db.Create(&newRepoConfig).Error; err != nil {
		return api.RepositoryResponse{}, DBErrorToApi(err)
	}

	var created api.RepositoryResponse
	ModelToApiFields(newRepoConfig, &created)
	created.URL = newRepo.URL
	created.Status = newRepo.Status

	return created, nil
}

func (r repositoryConfigDaoImpl) BulkCreate(newRepositories []api.RepositoryRequest) ([]api.RepositoryBulkCreateResponse, error) {
	var result []api.RepositoryBulkCreateResponse

	err := r.db.Transaction(func(tx *gorm.DB) error {
		var err error
		result, err = r.bulkCreate(tx, newRepositories)
		return err
	})
	return result, err
}

func (r repositoryConfigDaoImpl) bulkCreate(tx *gorm.DB, newRepositories []api.RepositoryRequest) ([]api.RepositoryBulkCreateResponse, error) {
	var dbErr error
	var response api.RepositoryResponse
	size := len(newRepositories)
	newRepoConfigs := make([]models.RepositoryConfiguration, size)
	newRepos := make([]models.Repository, size)
	result := make([]api.RepositoryBulkCreateResponse, size)

	tx.SavePoint("beforecreate")
	for i := 0; i < size; i++ {
		if newRepositories[i].OrgID != nil {
			newRepoConfigs[i].OrgID = *(newRepositories[i].OrgID)
		}
		if newRepositories[i].AccountID != nil {
			newRepoConfigs[i].AccountID = *(newRepositories[i].AccountID)
		}
		ApiFieldsToModel(newRepositories[i], &newRepoConfigs[i], &newRepos[i])

		if err := tx.Where("url = ?", newRepos[i].URL).FirstOrCreate(&newRepos[i]).Error; err != nil {
			dbErr = DBErrorToApi(err)
			errMsg := dbErr.Error()
			result[i] = api.RepositoryBulkCreateResponse{
				ErrorMsg:   errMsg,
				Repository: nil,
			}
			tx.RollbackTo("beforecreate")
			continue
		}

		newRepoConfigs[i].RepositoryUUID = newRepos[i].UUID
		if err := tx.Create(&newRepoConfigs[i]).Error; err != nil {
			dbErr = DBErrorToApi(err)
			errMsg := dbErr.Error()
			result[i] = api.RepositoryBulkCreateResponse{
				ErrorMsg:   errMsg,
				Repository: nil,
			}
			tx.RollbackTo("beforecreate")
			continue
		}

		ModelToApiFields(newRepoConfigs[i], &response)
		response.URL = newRepos[i].URL
		response.Status = newRepos[i].Status
		if dbErr == nil {
			result[i] = api.RepositoryBulkCreateResponse{
				ErrorMsg:   "",
				Repository: &response,
			}
		}
	}

	if dbErr != nil {
		for i := 0; i < size; i++ {
			result[i].Repository = nil
		}
	}

	return result, dbErr
}

func (r repositoryConfigDaoImpl) List(
	OrgID string,
	pageData api.PaginationData,
	filterData api.FilterData,
) (api.RepositoryCollectionResponse, int64, error) {
	var totalRepos int64
	repoConfigs := make([]models.RepositoryConfiguration, 0)

	filteredDB := r.db

	filteredDB = filteredDB.Where("org_id = ?", OrgID).
		Joins("inner join repositories on repository_configurations.repository_uuid = repositories.uuid")

	if filterData.AvailableForArch != "" {
		filteredDB = filteredDB.Where("arch = ? OR arch = ''", filterData.AvailableForArch)
	}
	if filterData.AvailableForVersion != "" {
		filteredDB = filteredDB.
			Where("? = any (versions) OR array_length(versions, 1) IS NULL", filterData.AvailableForVersion)
	}

	if filterData.Search != "" {
		containsSearch := "%" + filterData.Search + "%"
		filteredDB = filteredDB.
			Where("name LIKE ? OR url LIKE ?", containsSearch, containsSearch)
	}

	if filterData.Arch != "" {
		arches := strings.Split(filterData.Arch, ",")
		filteredDB = filteredDB.Where("arch IN ?", arches)
	}

	if filterData.Version != "" {
		versions := strings.Split(filterData.Version, ",")
		orGroup := r.db.Where("? = any (versions)", versions[0])
		for i := 1; i < len(versions); i++ {
			orGroup = orGroup.Or("? = any (versions)", versions[i])
		}
		filteredDB = filteredDB.Where(orGroup)
	}

	order := convertSortByToSQL(pageData.SortBy)

	filteredDB.Order(order).Find(&repoConfigs).Count(&totalRepos)
	filteredDB.Preload("Repository").Limit(pageData.Limit).Offset(pageData.Offset).Find(&repoConfigs)

	if filteredDB.Error != nil {
		return api.RepositoryCollectionResponse{}, totalRepos, filteredDB.Error
	}
	repos := convertToResponses(repoConfigs)
	return api.RepositoryCollectionResponse{Data: repos}, totalRepos, nil
}

func (r repositoryConfigDaoImpl) Fetch(orgID string, uuid string) (api.RepositoryResponse, error) {
	repo := api.RepositoryResponse{}
	repoConfig, err := r.fetchRepoConfig(orgID, uuid)

	if err != nil {
		return repo, err
	}
	ModelToApiFields(repoConfig, &repo)
	return repo, err
}

func (r repositoryConfigDaoImpl) fetchRepoConfig(orgID string, uuid string) (models.RepositoryConfiguration, error) {
	found := models.RepositoryConfiguration{}
	result := r.db.
		Preload("Repository").
		Where("text(UUID) = ? AND ORG_ID = ?", uuid, orgID).
		First(&found)

	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return found, &Error{NotFound: true, Message: "Could not find repository with UUID " + uuid}
		} else {
			return found, DBErrorToApi(result.Error)
		}
	}
	return found, nil
}

func (r repositoryConfigDaoImpl) Update(orgID string, uuid string, repoParams api.RepositoryRequest) error {
	var repo models.Repository
	var repoConfig models.RepositoryConfiguration
	var err error

	repoConfig, err = r.fetchRepoConfig(orgID, uuid)
	if err != nil {
		return err
	}
	ApiFieldsToModel(repoParams, &repoConfig, &repo)

	// If URL is included in params, search for existing
	// Repository record, or create a new one.
	// Then replace existing Repository/RepoConfig association.
	if repoParams.URL != nil {
		err = r.db.FirstOrCreate(&repo, "url = ?", *repoParams.URL).Error
		if err != nil {
			return DBErrorToApi(err)
		}
		repoConfig.RepositoryUUID = repo.UUID
	}

	repoConfig.Repository = models.Repository{}
	if err := r.db.Model(&repoConfig).Updates(repoConfig.MapForUpdate()).Error; err != nil {
		return DBErrorToApi(err)
	}
	return nil
}

// SavePublicRepos saves a list of urls and marks them as "Public"
//  This is meant for the list of repositories that are preloaded for all
//  users.
func (r repositoryConfigDaoImpl) SavePublicRepos(urls []string) error {
	var repos []models.Repository

	for i := 0; i < len(urls); i++ {
		repos = append(repos, models.Repository{URL: urls[i], Public: true})
	}
	result := r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "url"}},
		DoNothing: true}).Create(&repos)
	return result.Error
}

func (r repositoryConfigDaoImpl) Delete(orgID string, uuid string) error {
	var repoConfig models.RepositoryConfiguration
	var err error

	if repoConfig, err = r.fetchRepoConfig(orgID, uuid); err != nil {
		return err
	}
	if err = r.db.Delete(&repoConfig).Error; err != nil {
		return err
	}
	return nil
}

func ApiFieldsToModel(apiRepo api.RepositoryRequest, repoConfig *models.RepositoryConfiguration, repo *models.Repository) {
	if apiRepo.Name != nil {
		repoConfig.Name = *apiRepo.Name
	}
	if apiRepo.DistributionArch != nil {
		repoConfig.Arch = *apiRepo.DistributionArch
	}
	if apiRepo.DistributionVersions != nil {
		repoConfig.Versions = *apiRepo.DistributionVersions
	}
	if apiRepo.URL != nil {
		repo.URL = *apiRepo.URL
	}
	if apiRepo.GpgKey != nil {
		repoConfig.GpgKey = *apiRepo.GpgKey
	}
	if apiRepo.MetadataVerification != nil {
		repoConfig.MetadataVerification = *apiRepo.MetadataVerification
	}
}

func ModelToApiFields(repoConfig models.RepositoryConfiguration, apiRepo *api.RepositoryResponse) {
	apiRepo.UUID = repoConfig.UUID
	apiRepo.PackageCount = repoConfig.Repository.PackageCount
	apiRepo.URL = repoConfig.Repository.URL
	apiRepo.Name = repoConfig.Name
	apiRepo.DistributionVersions = repoConfig.Versions
	apiRepo.DistributionArch = repoConfig.Arch
	apiRepo.AccountID = repoConfig.AccountID
	apiRepo.OrgID = repoConfig.OrgID
	apiRepo.Status = repoConfig.Repository.Status
	apiRepo.GpgKey = repoConfig.GpgKey
	apiRepo.MetadataVerification = repoConfig.MetadataVerification

	if repoConfig.Repository.LastIntrospectionTime != nil {
		apiRepo.LastIntrospectionTime = repoConfig.Repository.LastIntrospectionTime.String()
	}
	if repoConfig.Repository.LastIntrospectionSuccessTime != nil {
		apiRepo.LastIntrospectionSuccessTime = repoConfig.Repository.LastIntrospectionSuccessTime.String()
	}
	if repoConfig.Repository.LastIntrospectionUpdateTime != nil {
		apiRepo.LastIntrospectionUpdateTime = repoConfig.Repository.LastIntrospectionUpdateTime.String()
	}
	if repoConfig.Repository.LastIntrospectionError != nil {
		apiRepo.LastIntrospectionError = *repoConfig.Repository.LastIntrospectionError
	}
}

func convertSortByToSQL(SortBy string) string {
	sqlOrderBy := ""

	sortByArray := strings.Split(SortBy, ",")
	lengthOfSortByParams := len(sortByArray)

	sortMap := map[string]string{
		"name":                  "name",
		"url":                   "url",
		"distribution_arch":     "arch",
		"distribution_versions": "array_to_string(versions, ',')",
		"package_count":         "package_count",
		"status":                "status",
	}

	for i := 0; i < lengthOfSortByParams; i++ {
		sortBy := sortByArray[i]

		split := strings.Split(sortBy, ":")
		ascOrDesc := " asc"

		if len(split) > 1 && split[1] == "desc" {
			ascOrDesc = " desc"
		}

		sortField, ok := sortMap[split[0]]

		// Only update if the sortMap above returns a valid value
		if ok {
			// Concatenate (e.g. "url desc," + "name" + " asc")
			sqlOrderBy = sqlOrderBy + sortField + ascOrDesc

			// Add a comma if this isn't the last item in the "sortByArray".
			if i+1 < lengthOfSortByParams {
				sqlOrderBy = sqlOrderBy + ", "
			}
		}
	}

	if sqlOrderBy == "" {
		sqlOrderBy = "name asc"
	}

	return sqlOrderBy
}

// Converts the database models to our response objects
func convertToResponses(repoConfigs []models.RepositoryConfiguration) []api.RepositoryResponse {
	repos := make([]api.RepositoryResponse, len(repoConfigs))
	for i := 0; i < len(repoConfigs); i++ {
		ModelToApiFields(repoConfigs[i], &repos[i])
	}
	return repos
}

func isTimeout(err error) bool {
	timeout, ok := err.(interface {
		Timeout() bool
	})
	if ok && timeout.Timeout() {
		return true
	}
	return false
}

func (r repositoryConfigDaoImpl) ValidateParameters(orgId string, params api.RepositoryValidationRequest, excludedUUIDS []string) (api.RepositoryValidationResponse, error) {
	var (
		err      error
		response api.RepositoryValidationResponse
	)

	response.Name = api.GenericAttributeValidationResponse{}
	if params.Name == nil {
		response.Name.Skipped = true
	} else {
		err = r.validateName(orgId, *params.Name, &response.Name, excludedUUIDS)
		if err != nil {
			return response, err
		}
	}

	response.URL = api.UrlValidationResponse{}
	if params.URL == nil {
		response.URL.Skipped = true
	} else {
		err = r.validateUrl(orgId, *params.URL, &response, excludedUUIDS)
		if err != nil {
			return response, err
		} else if response.URL.Valid {
			repomd := r.validateMetadataPresence(*params.URL, &response)
			if response.URL.MetadataPresent {
				r.checkSignaturePresent(&params, repomd, &response)
			}
		}
	}
	return response, err
}

func (r repositoryConfigDaoImpl) validateName(orgId string, name string, response *api.GenericAttributeValidationResponse, excludedUUIDS []string) error {
	if name == "" {
		response.Valid = false
		response.Error = "Name cannot be blank"
	} else {
		found := models.RepositoryConfiguration{}
		query := r.db.Where("name = ? AND ORG_ID = ?", name, orgId)
		if len(excludedUUIDS) != 0 {
			query = query.Where("repository_configurations.uuid NOT IN ?", excludedUUIDS)
		}
		result := query.Find(&found)
		if result.Error != nil {
			response.Valid = false
			return result.Error
		} else if found.UUID != "" {
			response.Valid = false
			response.Error = fmt.Sprintf("A repository with the name '%s' already exists.", name)
		} else {
			response.Valid = true
		}
	}
	return nil
}

func (r repositoryConfigDaoImpl) validateUrl(orgId string, url string, response *api.RepositoryValidationResponse, excludedUUIDS []string) error {
	if url == "" {
		response.URL.Valid = false
		response.URL.Error = "URL cannot be blank"
	} else {
		found := models.RepositoryConfiguration{}
		query := r.db.Preload("Repository").
			Joins("inner join repositories on repository_configurations.repository_uuid = repositories.uuid").
			Where("Repositories.URL = ? AND ORG_ID = ?", url, orgId)
		if len(excludedUUIDS) != 0 {
			query = query.Where("repository_configurations.uuid NOT IN ?", excludedUUIDS)
		}
		result := query.Find(&found)
		if result.Error != nil {
			response.URL.Valid = false
			return result.Error
		} else if found.UUID != "" {
			response.URL.Valid = false
			response.URL.Error = fmt.Sprintf("A repository with the URL '%s' already exists.", url)
		} else {
			response.URL.Valid = true
		}
	}
	return nil
}

func (r repositoryConfigDaoImpl) validateMetadataPresence(url string, response *api.RepositoryValidationResponse) *string {
	repomd, code, err := r.extResDao.FetchRepoMd(url)
	if err != nil {
		response.URL.HTTPCode = code
		if isTimeout(err) {
			response.URL.Error = fmt.Sprintf("Error fetching YUM metadata: %s", "Timeout occurred")
		} else {
			response.URL.Error = fmt.Sprintf("Error fetching YUM metadata: %s", err.Error())
		}
		response.URL.MetadataPresent = false
		return nil
	} else {
		response.URL.HTTPCode = code
		response.URL.MetadataPresent = code >= 200 && code < 300
		return repomd
	}
}

func (r repositoryConfigDaoImpl) checkSignaturePresent(request *api.RepositoryValidationRequest, repomd *string, response *api.RepositoryValidationResponse) {
	if request.GPGKey == nil || *request.GPGKey == "" {
		response.GPGKey.Skipped = true
		response.GPGKey.Valid = true
	} else {
		_, err := LoadGpgKey(request.GPGKey)
		if err == nil {
			response.GPGKey.Valid = true
		} else {
			response.GPGKey.Valid = false
			response.GPGKey.Error = fmt.Sprintf("Error loading GPG Key: %s.  Is this a valid GPG Key?", err.Error())
		}
	}

	sig, _, err := r.extResDao.FetchSignature(*request.URL)
	if err != nil || sig == nil {
		response.URL.MetadataSignaturePresent = false
	} else {
		response.URL.MetadataSignaturePresent = true
		if response.GPGKey.Valid && !response.GPGKey.Skipped && request.MetadataVerification { // GPG key is valid & signature present, so validate the signature
			sigErr := ValidateSignature(repomd, sig, request.GPGKey)
			if sigErr == nil {
				response.GPGKey.Valid = true
			} else if response.GPGKey.Error == "" {
				response.GPGKey.Valid = false
				response.GPGKey.Error = fmt.Sprintf("Error validating signature: %s. Is this the correct GPG Key?", sigErr.Error())
			}
		}
	}
}

func LoadGpgKey(gpgKey *string) (openpgp.EntityList, error) {
	keyRing, err := openpgp.ReadArmoredKeyRing(strings.NewReader(*gpgKey))
	return keyRing, err
}

func ValidateSignature(signedFile, sig, gpgKey *string) error {
	keyRing, err := LoadGpgKey(gpgKey)
	if err != nil {
		return err
	}

	_, err = openpgp.CheckArmoredDetachedSignature(keyRing, strings.NewReader(*signedFile), strings.NewReader(*sig), nil)
	if err != nil {
		return err
	}
	return nil
}
