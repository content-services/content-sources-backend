package handler

import (
	"encoding/json"
	"net/http"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/event"
	"github.com/content-services/content-sources-backend/pkg/rbac"
	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog/log"
)

type AdminNotificationsHandler struct{}

func checkAdminNotificationsAccessible(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if err := CheckAdminNotificationsAccessible(c.Request().Context()); err != nil {
			return err
		}
		return next(c)
	}
}

func RegisterAdminNotificationsRoutes(engine *echo.Group) {
	if engine == nil {
		panic("engine is nil")
	}

	h := AdminNotificationsHandler{}
	addRepoRoute(engine, http.MethodPost, "/admin/notifications/test/",
		h.sendTestNotification, rbac.RbacVerbWrite, checkAdminNotificationsAccessible)
}

func (h *AdminNotificationsHandler) sendTestNotification(c echo.Context) error {
	var req api.AdminSendTestNotificationRequest
	if err := c.Bind(&req); err != nil {
		return ce.NewErrorResponse(http.StatusBadRequest, "Error binding parameters", err.Error())
	}

	if len(req.Notification) == 0 || string(req.Notification) == "null" {
		return ce.NewErrorResponse(http.StatusBadRequest, "notification is required", "")
	}

	_, orgID := getAccountIdOrgId(c)
	log.Error().Msg(req.Topic)
	if req.Topic == config.Get().Options.LightwellBridgeTopic {
		events := []event.NotificationEvent{}
		err := json.Unmarshal([]byte(req.Notification), &events)
		if err != nil {
			return ce.NewErrorResponse(http.StatusBadRequest, "Error binding parameters", err.Error())
		}
		event.SendLightwellAdvisoryCreatedEvent(event.LightwellAdvisoryCreated, events)
		log.Error().Msg("Sent lightwell advisory created event")
		return c.NoContent(http.StatusOK)
	} else { // assume notification
		var body struct {
			OrgID string `json:"org_id"`
		}
		if err := json.Unmarshal(req.Notification, &body); err != nil {
			return ce.NewErrorResponse(http.StatusBadRequest, "Error parsing notification", err.Error())
		}
		if body.OrgID != "" && body.OrgID != orgID {
			return ce.NewErrorResponse(http.StatusForbidden, "org_id in notification does not match your identity", "")
		}

		sent, err := event.SendTestNotification(orgID, req.Notification)
		if err != nil {
			return ce.NewErrorResponse(http.StatusInternalServerError,
				"Error sending test notification", err.Error())
		}
		log.Error().Msg("Sent notification event")
		return c.JSONBlob(http.StatusOK, sent)
	}
}
