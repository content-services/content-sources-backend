package handler

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/clients/feature_service_client"
	"github.com/content-services/content-sources-backend/pkg/clients/terms_service_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/test"
	"github.com/labstack/echo/v4"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequireTermsAcceptance(t *testing.T) {
	networkFeatures := []string{"lightwell-network"}
	networkEvents := []string{"network"}
	user := &identity.User{Username: "user"}
	serviceAccount := &identity.ServiceAccount{Username: "service-account"}
	for _, tc := range []struct {
		name           string
		disabled       bool
		inaccessible   bool
		user           *identity.User
		serviceAccount *identity.ServiceAccount
		login          string
		checkFeatures  bool
		features       []string
		featureErr     error
		checkTerms     bool
		events         []string
		termsErr       error
		noMapping      bool
		blocked        bool
	}{
		{name: "feature disabled", disabled: true, user: user},
		{name: "feature inaccessible", inaccessible: true, user: user},
		{name: "internal user", user: &identity.User{Username: "internal", Internal: true}},
		{name: "no identity login"},
		{name: "empty user login", user: &identity.User{}},
		{name: "feature service failure", user: user, checkFeatures: true, featureErr: errors.New("feature service unavailable")},
		{name: "clearinghouse bypass", user: user, checkFeatures: true, features: []string{"lightwell-network", config.LightwellClearinghouseFeature}},
		{name: "terms service failure", user: user, login: "user", checkFeatures: true, features: networkFeatures, checkTerms: true, termsErr: errors.New("terms service unavailable")},
		{name: "terms accepted", user: user, login: "user", checkFeatures: true, features: networkFeatures, checkTerms: true},
		{name: "terms required", user: user, login: "user", checkFeatures: true, features: networkFeatures, checkTerms: true, events: networkEvents, blocked: true},
		{name: "unentitled events ignored", user: user, login: "user", checkFeatures: true, features: networkFeatures, checkTerms: true, events: []string{"academic"}},
		{name: "no entitled features", user: user, login: "user", checkFeatures: true, checkTerms: true, events: networkEvents},
		{name: "partially entitled events", user: user, login: "user", checkFeatures: true, features: networkFeatures, checkTerms: true, events: []string{"academic", "network"}, blocked: true},
		{name: "unmapped events required", user: user, login: "user", checkFeatures: true, checkTerms: true, events: []string{"unmapped"}, blocked: true},
		{name: "no mapping requires all events", user: user, login: "user", checkFeatures: true, checkTerms: true, events: networkEvents, noMapping: true, blocked: true},
		{name: "service account", serviceAccount: serviceAccount, login: "service-account", checkFeatures: true, features: networkFeatures, checkTerms: true, events: networkEvents, blocked: true},
		{name: "empty user falls back to service account", user: &identity.User{}, serviceAccount: serviceAccount, login: "service-account", checkFeatures: true, features: networkFeatures, checkTerms: true, events: networkEvents, blocked: true},
		{name: "user login takes precedence", user: user, serviceAccount: serviceAccount, login: "user", checkFeatures: true, features: networkFeatures, checkTerms: true, events: networkEvents, blocked: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			previousConfig := config.LoadedConfig
			t.Cleanup(func() { config.LoadedConfig = previousConfig })
			config.LoadedConfig.Loaded = true
			config.LoadedConfig.Features.LightwellTerms = config.Feature{Enabled: !tc.disabled}
			if tc.inaccessible {
				config.LoadedConfig.Features.LightwellTerms.Organizations = &[]string{"other-org"}
			}
			config.LoadedConfig.Clients.TermsService.EventFeatureMap = "network:lightwell-network,academic:lightwell-research-institutions"
			if tc.noMapping {
				config.LoadedConfig.Clients.TermsService.EventFeatureMap = ""
			}

			fs := feature_service_client.NewMockFeatureServiceClient(t)
			ts := terms_service_client.NewMockTermsServiceClient(t)
			if tc.checkFeatures {
				fs.On("GetEntitledFeatures", test.MockCtx(), "org").Return(tc.features, tc.featureErr).Once()
			}
			if tc.checkTerms {
				ts.On("GetRequiredEvents", test.MockCtx(), tc.login).Return(tc.events, tc.termsErr).Once()
			}

			router := echo.New()
			router.HTTPErrorHandler = config.CustomHTTPErrorHandler
			calls := 0
			router.GET("/guarded", func(c echo.Context) error {
				calls++
				return c.NoContent(http.StatusNoContent)
			}, RequireTermsAcceptance(ts, fs))
			req := httptest.NewRequest(http.MethodGet, "/guarded", nil)
			req = req.WithContext(identity.WithIdentity(req.Context(), identity.XRHID{
				Identity: identity.Identity{
					OrgID: "org", Internal: identity.Internal{OrgID: "org"},
					User: tc.user, ServiceAccount: tc.serviceAccount,
				},
			}))
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if tc.blocked {
				assert.Equal(t, http.StatusPreconditionRequired, rec.Code)
				assert.Zero(t, calls, "guarded handler must not execute")
				assert.JSONEq(t, `{"errors":[{"status":428,"title":"Terms acceptance required","detail":"You must accept the Lightwell terms of use before accessing this resource. Visit https://console.redhat.com/lightwell to accept."}]}`, rec.Body.String())
			} else {
				assert.Equal(t, http.StatusNoContent, rec.Code)
				assert.Equal(t, 1, calls)
			}
		})
	}
}

func TestRequireTermsAcceptancePropagatesHandlerError(t *testing.T) {
	previousConfig := config.LoadedConfig
	t.Cleanup(func() { config.LoadedConfig = previousConfig })
	config.LoadedConfig.Loaded = true
	config.LoadedConfig.Features.LightwellTerms = config.Feature{}
	router := echo.New()
	ctx := router.NewContext(httptest.NewRequest(http.MethodGet, "/", nil), httptest.NewRecorder())
	expected := ce.NewErrorResponse(http.StatusBadRequest, "Invalid request", "Handler validation failed")
	middleware := RequireTermsAcceptance(nil, nil)
	err := middleware(func(echo.Context) error { return expected })(ctx)
	require.Equal(t, expected, err)
}
