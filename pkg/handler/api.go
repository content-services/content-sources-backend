package handler

import "C"
import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	spec_api "github.com/content-services/content-sources-backend/api"
	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/event/producer"
	"github.com/labstack/echo/v4"
	"github.com/openlyinc/pointy"
	"github.com/rs/zerolog/log"
)

const DefaultOffset = 0
const DefaultLimit = 100
const DefaultSortBy = ""
const DefaultSearch = ""
const DefaultArch = ""
const DefaultVersion = ""
const DefaultAvailableForArch = ""
const DefaultAvailableForVersion = ""
const MaxLimit = 200
const ApiVersion = "1.0"
const ApiVersionMajor = "1"

// nolint: lll
// @title ContentSourcesBackend
// DO NOT EDIT version MANUALLY - this variable is modified by generate_docs.sh
// @version  v1.0.0
// @description API of the Content Sources application on [console.redhat.com](https://console.redhat.com)
// @description
// @license.name Apache 2.0
// @license.url https://www.apache.org/licenses/LICENSE-2.0
// @Host api.example.com
// @BasePath /api/content-sources/v1.0/
// @query.collection.format multi
// @securityDefinitions.apikey RhIdentity
// @in header
// @name x-rh-identity

func RegisterRoutes(engine *echo.Echo) {
	var (
		err               error
		kafkaProducer     *kafka.Producer
		introspectRequest producer.IntrospectRequest
	)
	pagedRpmInsertsLimit := config.Get().Options.PagedRpmInsertsLimit
	engine.GET("/ping", ping)
	paths := []string{fullRootPath(), majorRootPath()}
	if kafkaProducer, err = producer.NewProducer(&config.Get().Kafka); err != nil {
		panic(err)
	}
	if introspectRequest, err = producer.NewIntrospectRequest(kafkaProducer); err != nil {
		panic(err)
	}
	for i := 0; i < len(paths); i++ {
		group := engine.Group(paths[i])
		group.GET("/ping", ping)
		group.GET("/openapi.json", openapi)

		daoRepo := dao.GetRepositoryConfigDao(db.DB)
		externalRepo := dao.GetExternalResourceDao()
		RegisterRepositoryRoutes(group, &daoRepo, &introspectRequest)
		RegisterRepositoryParameterRoutes(group, &daoRepo, &externalRepo)

		daoRpm := dao.GetRpmDao(db.DB, &dao.RpmDaoOptions{
			PagedRpmInsertsLimit: pointy.Int(pagedRpmInsertsLimit),
		})
		RegisterRepositoryRpmRoutes(group, &daoRpm)
	}

	data, err := json.MarshalIndent(engine.Routes(), "", "  ")
	if err == nil {
		log.Debug().Msg(string(data))
	}
}

func ping(c echo.Context) error {
	return c.JSON(200, echo.Map{
		"message": "pong",
	})
}

func openapi(c echo.Context) error {
	var foo, err = spec_api.Openapi()
	if err != nil {
		return err
	}
	return c.JSONBlob(200, foo)
}

func rootPrefix() string {
	pathPrefix, present := os.LookupEnv("PATH_PREFIX")
	if !present {
		pathPrefix = "api"
	}

	appName, present := os.LookupEnv("APP_NAME")
	if !present {
		appName = config.DefaultAppName
	}
	return filepath.Join("/", pathPrefix, appName)
}

func fullRootPath() string {
	return filepath.Join(rootPrefix(), "v"+ApiVersion)
}
func majorRootPath() string {
	return filepath.Join(rootPrefix(), "v"+ApiVersionMajor)
}

func createLink(c echo.Context, offset int) string {
	req := c.Request()
	q := req.URL.Query()
	page := ParsePagination(c)
	filters := ParseFilters(c)

	q.Set("limit", strconv.Itoa(page.Limit))
	q.Set("offset", strconv.Itoa(offset))

	if filters.Search != "" {
		q.Set("search", filters.Search)
	}
	if filters.Arch != "" {
		q.Set("arch", filters.Arch)
	}
	if filters.Version != "" {
		q.Set("version", filters.Version)
	}
	if filters.AvailableForArch != "" {
		q.Set("available_for_arch", filters.AvailableForArch)
	}

	if filters.AvailableForVersion != "" {
		q.Set("available_for_version", filters.AvailableForVersion)
	}

	params, _ := url.PathUnescape(q.Encode())
	return fmt.Sprintf("%v?%v", req.URL.Path, params)
}

func httpCodeForError(err error) int {
	daoError, ok := err.(*dao.Error)
	if ok {
		if daoError.NotFound {
			return http.StatusNotFound
		} else if daoError.BadValidation {
			return http.StatusBadRequest
		} else {
			return http.StatusInternalServerError
		}
	} else {
		return http.StatusInternalServerError
	}
}

// setCollectionResponseMetadata determines metadata of collection response based on context and collection size.
// Returns collection response with updated metadata.
func setCollectionResponseMetadata(collection api.CollectionMetadataSettable, c echo.Context, totalCount int64) api.CollectionMetadataSettable {
	page := ParsePagination(c)
	var lastPage int
	if int(totalCount) > 0 && (int(totalCount)%page.Limit) == 0 {
		lastPage = int(totalCount) - page.Limit
	} else {
		lastPage = int(totalCount) - int(totalCount)%page.Limit
	}
	links := api.Links{
		First: createLink(c, 0),
		// 100/page.Limit - (100%10)

		Last: createLink(c, lastPage),
	}
	if page.Offset+page.Limit < int(totalCount) {
		links.Next = createLink(c, page.Offset+page.Limit)
	}
	if page.Offset-page.Limit >= 0 {
		links.Prev = createLink(c, page.Offset-page.Limit)
	}

	collection.SetMetadata(api.ResponseMetadata{
		Count:  totalCount,
		Limit:  page.Limit,
		Offset: page.Offset,
	}, links)
	return collection
}

func ParsePagination(c echo.Context) api.PaginationData {
	pageData := api.PaginationData{Limit: DefaultLimit, Offset: DefaultOffset, SortBy: DefaultSortBy}
	err := echo.QueryParamsBinder(c).
		Int("limit", &pageData.Limit).
		Int("offset", &pageData.Offset).
		String("sort_by", &pageData.SortBy).
		BindError()

	if err != nil {
		log.Error().Err(err).Msg("Failed to bind pagination.")
	}

	if pageData.SortBy == DefaultSortBy {
		err = c.Request().ParseForm()
		if err != nil {
			log.Error().Err(err).Msg("Failed to bind pagination.")
		}
		q := c.Request().Form
		pageData.SortBy = strings.Join(q["sort_by[]"], ",")
	}

	if pageData.Limit > MaxLimit {
		pageData.Limit = MaxLimit
	}
	return pageData
}

func ParseFilters(c echo.Context) api.FilterData {
	filterData := api.FilterData{
		Search:              DefaultSearch,
		Arch:                DefaultArch,
		Version:             DefaultVersion,
		AvailableForArch:    DefaultAvailableForArch,
		AvailableForVersion: DefaultAvailableForVersion,
	}
	err := echo.QueryParamsBinder(c).
		String("search", &filterData.Search).
		String("arch", &filterData.Arch).
		String("version", &filterData.Version).
		String("available_for_arch", &filterData.AvailableForArch).
		String("available_for_version", &filterData.AvailableForVersion).
		String("name", &filterData.Name).
		String("url", &filterData.URL).
		BindError()

	if err != nil {
		log.Error().Err(err).Msg("Error parsing filters")
	}

	return filterData
}
