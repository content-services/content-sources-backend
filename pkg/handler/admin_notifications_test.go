package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/event"
	"github.com/content-services/content-sources-backend/pkg/middleware"
	"github.com/content-services/content-sources-backend/pkg/seeds"
	test_handler "github.com/content-services/content-sources-backend/pkg/test/handler"
	"github.com/labstack/echo/v4"
	echo_middleware "github.com/labstack/echo/v4/middleware"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type AdminNotificationsSuite struct {
	suite.Suite
}

func TestAdminNotificationsSuite(t *testing.T) {
	suite.Run(t, new(AdminNotificationsSuite))
}

func (suite *AdminNotificationsSuite) serveAdminNotificationsRouter(req *http.Request, enabled bool, authorized bool) (int, []byte, error) {
	router := echo.New()
	router.Use(echo_middleware.RequestIDWithConfig(echo_middleware.RequestIDConfig{
		TargetHeader: "x-rh-insights-request-id",
	}))
	router.Use(middleware.WrapMiddlewareWithSkipper(identity.EnforceIdentity, middleware.SkipMiddleware))
	router.HTTPErrorHandler = config.CustomHTTPErrorHandler
	pathPrefix := router.Group(api.FullRootPath())

	if enabled {
		config.Get().Features.AdminNotifications.Enabled = true
	} else {
		config.Get().Features.AdminNotifications.Enabled = false
	}
	if authorized {
		config.Get().Features.AdminNotifications.Accounts = &[]string{test_handler.MockAccountNumber}
	} else {
		config.Get().Features.AdminNotifications.Accounts = &[]string{seeds.RandomAccountId()}
	}

	RegisterAdminNotificationsRoutes(pathPrefix)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	response := rr.Result()
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	return response.StatusCode, body, err
}

func testNotificationPayload(orgID string) json.RawMessage {
	return json.RawMessage(`{
		"bundle": "rhel",
		"application": "repositories",
		"event_type": "repository-created",
		"timestamp": "2026-07-31T18:52:01Z",
		"org_id": "` + orgID + `",
		"context": {},
		"events": [{"metadata": {}, "payload": {"name": "test-repo", "url": "https://example.com/repo"}}]
	}`)
}

func (suite *AdminNotificationsSuite) TestSendTestNotificationDisabled() {
	t := suite.T()

	body, err := json.Marshal(api.AdminSendTestNotificationRequest{
		Notification: testNotificationPayload(test_handler.MockOrgId),
	})
	assert.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, api.FullRootPath()+"/admin/notifications/test/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, respBody, err := suite.serveAdminNotificationsRouter(req, false, false)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Contains(t, string(respBody), "Admin notifications feature is disabled")
}

func (suite *AdminNotificationsSuite) TestSendTestNotificationNotAccessible() {
	t := suite.T()

	body, err := json.Marshal(api.AdminSendTestNotificationRequest{
		Notification: testNotificationPayload(test_handler.MockOrgId),
	})
	assert.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, api.FullRootPath()+"/admin/notifications/test/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, respBody, err := suite.serveAdminNotificationsRouter(req, true, false)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Contains(t, string(respBody), "Neither the user nor account is allowed")
}

func (suite *AdminNotificationsSuite) TestSendTestNotificationMissingNotification() {
	t := suite.T()

	body, err := json.Marshal(api.AdminSendTestNotificationRequest{})
	assert.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, api.FullRootPath()+"/admin/notifications/test/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, respBody, err := suite.serveAdminNotificationsRouter(req, true, true)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Contains(t, string(respBody), "notification is required")
}

func (suite *AdminNotificationsSuite) TestSendTestNotificationOrgMismatch() {
	t := suite.T()

	body, err := json.Marshal(api.AdminSendTestNotificationRequest{
		Notification: testNotificationPayload("wrong-org-id"),
	})
	assert.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, api.FullRootPath()+"/admin/notifications/test/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, respBody, err := suite.serveAdminNotificationsRouter(req, true, true)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusForbidden, code)
	assert.Contains(t, string(respBody), "org_id in notification does not match your identity")
}

func (suite *AdminNotificationsSuite) TestSendTestNotificationNoClient() {
	t := suite.T()

	origProducer := config.Get().NotificationsProducer
	config.Get().NotificationsProducer = nil
	defer func() { config.Get().NotificationsProducer = origProducer }()

	body, err := json.Marshal(api.AdminSendTestNotificationRequest{
		Notification: testNotificationPayload(test_handler.MockOrgId),
	})
	assert.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, api.FullRootPath()+"/admin/notifications/test/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, respBody, err := suite.serveAdminNotificationsRouter(req, true, true)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusInternalServerError, code)
	assert.Contains(t, string(respBody), "notifications client is not configured")
}

func TestPackageEcosystemType(t *testing.T) {
	assert.Equal(t, "maven", packageEcosystemType("org.example:lib-a"))
	assert.Equal(t, "maven", packageEcosystemType("ch.qos.logback:logback-classic"))
	assert.Equal(t, "python", packageEcosystemType("requests"))
	assert.Equal(t, "python", packageEcosystemType("django-rest-framework"))
}

func TestEnrichBridgeEventsWithChecksums(t *testing.T) {
	const testSHA = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expected := "/java/remediated/com/example/lib-a/1.0.1.rhlw-00001/lib-a-1.0.1.rhlw-00001.jar.sha256"
		if r.URL.Path == expected {
			fmt.Fprint(w, testSHA)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	origOrigin := config.Get().Clients.Pulp.ContentOrigin
	origPrefix := config.Get().Clients.Pulp.ContentPathPrefix
	origUser := config.Get().Clients.Lightwell.Username
	origPass := config.Get().Clients.Lightwell.Password
	config.Get().Clients.Pulp.ContentOrigin = srv.URL
	config.Get().Clients.Pulp.ContentPathPrefix = ""
	config.Get().Clients.Lightwell.Username = ""
	config.Get().Clients.Lightwell.Password = ""
	defer func() {
		config.Get().Clients.Pulp.ContentOrigin = origOrigin
		config.Get().Clients.Pulp.ContentPathPrefix = origPrefix
		config.Get().Clients.Lightwell.Username = origUser
		config.Get().Clients.Lightwell.Password = origPass
	}()

	events := []event.NotificationEvent{
		{
			Metadata: map[string]any{},
			Payload: event.LightwellPackagePayload{
				PackageName: "com.example:lib-a",
				Releases: []event.LightwellReleasePayload{
					{
						ReleaseNames: []event.LightwellReleaseName{{Name: "1.0.1.rhlw-00001"}},
						RelatedCVE:   []event.LightwellCVEPayload{{CVE: "CVE-2026-0001"}},
					},
				},
			},
		},
	}

	enrichBridgeEventsWithChecksums(context.Background(), events)

	payload, ok := events[0].Payload.(event.LightwellPackagePayload)
	require.True(t, ok)
	require.NotEmpty(t, payload.Releases)
	assert.Equal(t, testSHA, payload.Releases[0].ArtifactChecksums["lib-a-1.0.1.rhlw-00001.jar"])
}
