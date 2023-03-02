package handler

import (
	"fmt"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/content-services/content-sources-backend/pkg/event/message"
	"github.com/content-services/content-sources-backend/pkg/external_repos"
	"github.com/go-playground/validator/v10"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// IntrospectHandler struct
type IntrospectHandler struct {
	Tx *gorm.DB
}

// NewIntrospectHandler creates a handler to process introspect request messages.
// db is the database connector.
func NewIntrospectHandler(db *gorm.DB) *IntrospectHandler {
	if db == nil {
		return nil
	}
	return &IntrospectHandler{
		Tx: db,
	}
}

// OnMessage processes the kafka message.
// msg is the message to be processed.
// Return nil if it is processed with success, else nil.
func (h *IntrospectHandler) OnMessage(msg *kafka.Message) error {
	var key = string(msg.Key)
	log.Debug().Msgf("IntrospectHandler.OnMessage was called; Key=%s", key)

	payload := &message.IntrospectRequestMessage{}
	if err := payload.UnmarshalJSON(msg.Value); err != nil {
		return fmt.Errorf("Error deserializing payload: %w", err)
	}

	// https://github.com/go-playground/validator
	// FIXME Wrong usage of validator library
	validate := validator.New()
	if err := validate.Var(payload.Url, "required,url"); err != nil {
		return err
	}

	newRpms, errs := external_repos.IntrospectUrl(payload.Url, true)
	if len(errs) > 0 {
		// Introspection failure isn't considered a message failure, as the message has been handled
		for i := 0; i < len(errs); i++ {
			log.Error().Err(errs[i]).Msgf("Error %v introspecting repository %v", i, payload.Url)
		}
	}
	log.Debug().Msgf("IntrospectionUrl returned %d new packages", newRpms)
	return nil
}
