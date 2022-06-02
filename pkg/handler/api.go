package handler

import "C"
import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"

	"github.com/content-services/content-sources-backend/docs"
	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog/log"
)

const DefaultOffset = 0
const DefaultLimit = 100
const DefaultAppName = "content_sources"
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
// @BasePath /api/content_sources/v1.0/
// @query.collection.format multi
// @securityDefinitions.apikey RhIdentity
// @in header
// @name x-rh-identity

func RegisterRoutes(engine *echo.Echo) {
	engine.GET("/ping", ping)
	paths := []string{fullRootPath(), majorRootPath()}
	for i := 0; i < len(paths); i++ {
		group := engine.Group(paths[i])
		group.GET("/ping", ping)
		group.GET("/openapi.json", openapi)
		RegisterRepositoryRoutes(group)
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
	var foo, err = docs.Openapi()
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
		appName = DefaultAppName
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

	q.Set("limit", strconv.Itoa(page.Limit))
	q.Set("offset", strconv.Itoa(offset))

	params, _ := url.PathUnescape(q.Encode())
	return fmt.Sprintf("%v?%v", req.URL.Path, params)
}

func collectionResponse(collection api.CollectionMetadataSettable, c echo.Context, totalCount int64) api.CollectionMetadataSettable {
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
	pageData := api.PaginationData{Limit: DefaultLimit, Offset: DefaultOffset}
	err := echo.QueryParamsBinder(c).
		Int("limit", &pageData.Limit).
		Int("offset", &pageData.Offset).
		BindError()

	if err != nil {
		log.Fatal().Err(err)
	}
	if pageData.Limit > MaxLimit {
		pageData.Limit = MaxLimit
	}
	return pageData
}
