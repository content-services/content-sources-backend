package api

import "encoding/json"

type AdminSendTestNotificationRequest struct {
	Notification json.RawMessage `json:"notification"`
}
