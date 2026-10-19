package lightwell

import (
	"context"

	lwdocs "github.com/content-services/content-sources-backend/api/lightwell"
	"github.com/content-services/content-sources-backend/pkg/clients/feature_service_client"
	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/clients/s3_client"
	"github.com/content-services/content-sources-backend/pkg/clients/terms_service_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/middleware"
	"github.com/content-services/content-sources-backend/pkg/rbac"
	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog/log"
)

const LightwellAPIPath = "/api/lightwell/v0.1"

// @title LightwellAPI
// @version v0.1.0
// @description API for Lightwell Network.
// @license.name Apache 2.0
// @license.url https://www.apache.org/licenses/LICENSE-2.0
// @Host console.redhat.com
// @BasePath /api/lightwell/v0.1
// @query.collection.format multi
// @securityDefinitions.apikey RhIdentity
// @in header
// @name x-rh-identity

func openapi(c echo.Context) error {
	data, err := lwdocs.Openapi()
	if err != nil {
		return err
	}
	return c.JSONBlob(200, data)
}

func RegisterRoutes(_ context.Context, engine *echo.Echo) {
	group := engine.Group(LightwellAPIPath)

	group.GET("/openapi.json", openapi)

	daoReg := dao.GetDaoRegistry(db.DB)
	fsClient, err := feature_service_client.NewFeatureServiceClient()
	if err != nil {
		panic(err)
	}

	RegisterLightwellRepositoryRoutes(group, daoReg)

	var termsMiddleware []echo.MiddlewareFunc
	if config.TermsServiceConfigured() {
		var tsClient terms_service_client.TermsServiceClient
		tsClient, err = terms_service_client.NewTermsServiceClient()
		if err != nil {
			log.Error().Err(err).Msg("failed to create terms service client; terms routes will not be registered")
		} else {
			RegisterLightwellTermsRoutes(group, &tsClient, &fsClient)
			termsMiddleware = append(termsMiddleware, middleware.RequireTermsAcceptance(tsClient, fsClient))
		}
	}

	RegisterLightwellAdvisoryRoutes(group, daoReg, &fsClient, termsMiddleware...)

	pulpClient := pulp_client.GetPulpClientWithDomain("")
	if config.Tang == nil {
		err = config.ConfigureTang()
		if err != nil {
			panic(err)
		}
	}
	if config.Tang != nil {
		RegisterLightwellPackageRoutes(group, daoReg, *config.Tang, pulpClient, &fsClient, termsMiddleware...)
	}

	var uploadS3 s3_client.S3Client
	if config.Get().Clients.Lightwell.S3.ClearinghouseUploads.Name == "" {
		log.Warn().Msg("clearinghouse uploads s3 not configured")
	} else {
		uploadS3, err = s3_client.NewS3Client(config.Get().Clients.Lightwell.S3.ClearinghouseUploads)
		if err != nil {
			panic(err)
		}
	}
	RegisterClearinghouseRoutes(group, daoReg.ClearinghouseUpload, uploadS3)
}

func addLightwellRoute(e *echo.Group, method string, path string, h echo.HandlerFunc, verb rbac.Verb, m ...echo.MiddlewareFunc) {
	e.Add(method, path, h, m...)
	// using repositories resource until lightwell resource is available
	rbac.ServicePermissions.Add(method, path, rbac.ResourceRepositories, verb)
}
