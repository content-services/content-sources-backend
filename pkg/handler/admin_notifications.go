package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/event"
	"github.com/content-services/content-sources-backend/pkg/external_repos"
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

	_, orgID := GetAccountIdOrgId(c)
	log.Error().Msg(req.Topic)
	if req.Topic == config.Get().Options.LightwellBridgeTopic {
		events := []event.NotificationEvent{}
		err := json.Unmarshal([]byte(req.Notification), &events)
		if err != nil {
			return ce.NewErrorResponse(http.StatusBadRequest, "Error binding parameters", err.Error())
		}

		if c.QueryParam("include_checksums") == "true" {
			enrichBridgeEventsWithChecksums(c.Request().Context(), events)
		}

		err = event.SendLightwellAdvisoryCreatedEvent(event.LightwellAdvisoryCreated, event.LightwellEventTypeJavaRemediated, events)
		if err != nil {
			return ce.NewErrorResponse(http.StatusBadRequest, "Error sending message", err.Error())
		}
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

func enrichBridgeEventsWithChecksums(ctx context.Context, events []event.NotificationEvent) {
	entries, err := external_repos.LoadLightwellAllowlist()
	if err != nil {
		log.Warn().Err(err).Msg("cannot load allowlist for checksum enrichment")
		return
	}

	entryByName := make(map[string]external_repos.LightwellAllowlistEntry, len(entries))
	for _, e := range entries {
		entryByName[e.Name] = e
	}

	httpClient := &http.Client{Timeout: 30 * time.Second}

	for i, ev := range events {
		payload, ok := ev.Payload.(event.LightwellPackagePayload)
		if !ok {
			raw, _ := json.Marshal(ev.Payload)
			var p event.LightwellPackagePayload
			if json.Unmarshal(raw, &p) != nil {
				continue
			}
			payload = p
		}

		for _, entry := range entries {
			if event.LightwellEventType(entry.Name) == "" {
				continue
			}

			hostname := config.Get().Clients.Pulp.LightwellContentOrigin
			if hostname == "" {
				hostname = config.Get().Clients.Pulp.ContentOrigin
			}
			contentBase, _ := url.JoinPath(hostname, config.Get().Clients.Pulp.ContentPathPrefix, entry.BasePath)
			contentBase = strings.TrimRight(contentBase, "/")

			var requests []event.ArtifactChecksumRequest
			for _, rel := range payload.Releases {
				for _, rn := range rel.ReleaseNames {
					requests = append(requests, event.ArtifactChecksumRequest{
						PackageName: payload.PackageName,
						Version:     rn.Name,
					})
				}
			}

			checksums := event.FetchArtifactChecksums(
				ctx, httpClient, contentBase, entry.Type,
				requests,
				config.Get().Clients.Lightwell.Username,
				config.Get().Clients.Lightwell.Password,
			)

			if len(checksums) > 0 {
				for j, rel := range payload.Releases {
					merged := make(map[string]string)
					for _, rn := range rel.ReleaseNames {
						key := event.ArtifactChecksumKey(payload.PackageName, rn.Name)
						for filename, sha := range checksums[key] {
							merged[filename] = sha
						}
					}
					if len(merged) > 0 {
						payload.Releases[j].ArtifactChecksums = merged
					}
				}
				events[i].Payload = payload
				break
			}
		}
	}
}
