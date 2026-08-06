package event

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/IBM/sarama"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// SendNotification sends a repository event to the notifications service using the Action format.
func SendNotification(orgID string, eventName EventName, repos []RepositoryPayload) {
	producer := config.Get().NotificationsProducer
	if producer != nil && len(repos) > 0 {
		events := make([]NotificationEvent, len(repos))
		for i, repo := range repos {
			events[i] = NotificationEvent{
				Metadata: map[string]any{},
				Payload:  repo,
			}
		}

		action := NotificationAction{
			Version:     NotificationVersion,
			Bundle:      NotificationBundle,
			Application: NotificationApplication,
			EventType:   eventName.String(),
			Timestamp:   time.Now().UTC().Format(time.RFC3339),
			OrgID:       orgID,
			Context:     map[string]any{},
			Events:      events,
		}

		msgBytes, err := json.Marshal(action)
		if err != nil {
			log.Error().Err(err).Msg("failed to marshal notification action")
			return
		}

		_, _, err = producer.Producer.SendMessage(&sarama.ProducerMessage{
			Topic: producer.Topic,
			Value: sarama.ByteEncoder(msgBytes),
		})
		if err != nil {
			log.Error().Err(err).Msg("notification message failed to send")
			return
		}
	} else if config.Get().Options.EnableNotifications {
		log.Warn().Msg("NotificationsProducer is nil")
	}
}

func SendTestNotification(orgID string, payload json.RawMessage) (json.RawMessage, error) {
	producer := config.Get().NotificationsProducer
	if producer == nil {
		return nil, fmt.Errorf("notifications client is not configured")
	}

	var body map[string]any
	if err := json.Unmarshal(payload, &body); err != nil {
		return nil, fmt.Errorf("failed to parse notification payload: %w", err)
	}

	body["org_id"] = orgID

	msgBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal notification payload: %w", err)
	}

	_, _, err = producer.Producer.SendMessage(&sarama.ProducerMessage{
		Topic: producer.Topic,
		Value: sarama.ByteEncoder(msgBytes),
	})
	if err != nil {
		return nil, fmt.Errorf("notification message failed to send: %v", err)
	}

	return msgBytes, nil
}

// SendTemplateEvent - Sends an event about a template to the patch service
func SendTemplateEvent(orgID string, eventName EventName, templates []TemplateEvent) {
	if config.Get().TemplateEventClient != nil && len(templates) > 0 {
		eventNameStr := eventName.String()
		newUUID, _ := uuid.NewRandom()
		e := cloudevents.NewEvent()
		e.SetSource("urn:redhat:source:console:app:repositories")
		e.SetID(newUUID.String())
		e.SetType("com.redhat.console.repositories." + eventNameStr)
		e.SetSubject("urn:redhat:subject:console:rhel:" + eventNameStr)
		e.SetTime(time.Now())
		e.SetExtension("redhatorgid", orgID)

		data := templates
		err := e.SetData(cloudevents.ApplicationJSON, data)

		if err != nil {
			log.Error().Err(err).Msg("failed to create cloudevents client")
			return
		}

		ctx := cloudevents.WithEncodingStructured(context.Background())
		// Send the event
		if result := config.Get().TemplateEventClient.Send(ctx, e); cloudevents.IsUndelivered(result) {
			log.Error().Msgf("Notification message failed to send: %v", result)
			return
		}
	}
}
