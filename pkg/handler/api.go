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
	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog/log"
)

const DefaultOffset = 0
const DefaultLimit = 100
const DefaultAppName = "content_sources"
const MaxLimit = 200
const ApiVersion = "1.0"

type CollectionMetadataSettable interface {
	setMetadata(meta ResponseMetadata, links Links)
}

type PaginationData struct {
	Limit  int `query:"limit" json:"limit" `  //Number of results to return
	Offset int `query:"offset" json:"offset"` //Offset into the total results
}

type ResponseMetadata struct {
	Limit  int   `query:"limit" json:"limit"`   //Limit of results used for the request
	Offset int   `query:"offset" json:"offset"` //Offset into results used for the request
	Count  int64 `json:"count"`                 //Total count of results
}

type Links struct {
	First string `json:"first"`          //Path to first page of results
	Next  string `json:"next,omitempty"` //Path to next page of results
	Prev  string `json:"prev,omitempty"` //Path to previous page of results
	Last  string `json:"last"`           //Path to last page of results
}

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

	group := engine.Group(rootRoute())
	group.GET("/ping", ping)
	group.GET("/openapi.json", openapi)
	RegisterRepositoryRoutes(group)

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

func rootRoute() string {
	pathPrefix, present := os.LookupEnv("PATH_PREFIX")
	if !present {
		pathPrefix = "api"
	}

	appName, present := os.LookupEnv("APP_NAME")
	if !present {
		appName = DefaultAppName
	}
	return filepath.Join(pathPrefix, appName, "v"+ApiVersion)
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

func collectionResponse(collection CollectionMetadataSettable, c echo.Context, totalCount int64) CollectionMetadataSettable {
	page := ParsePagination(c)
	var lastPage int
	if (int(totalCount) % page.Limit) == 0 {
		lastPage = int(totalCount) - page.Limit
	} else {
		lastPage = int(totalCount) - int(totalCount)%page.Limit
	}
	links := Links{
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

	collection.setMetadata(ResponseMetadata{
		Count:  totalCount,
		Limit:  page.Limit,
		Offset: page.Offset,
	}, links)
	return collection
}

func ParsePagination(c echo.Context) PaginationData {
	pageData := PaginationData{Limit: DefaultLimit, Offset: DefaultOffset}
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
