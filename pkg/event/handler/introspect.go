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

type IntrospectHandler struct {
	Tx *gorm.DB
}

func (h *IntrospectHandler) OnMessage(msg *kafka.Message) error {
	var key = string(msg.Key)
	log.Debug().Msgf("OnMessage was called; Key=%s", key)

	payload := &message.IntrospectRequestMessage{}
	if err := payload.UnmarshalJSON(msg.Value); err != nil {
		return fmt.Errorf("Error deserializing payload: %w", err)
	}

	// https://github.com/go-playground/validator
	validate := validator.New()
	if err := validate.Var(payload.Url, "required,url"); err != nil {
		return err
	}

	newRpms, errs := external_repos.IntrospectUrl(payload.Url, true)
	if len(errs) > 0 {
		return errs[0]
	}
	log.Debug().Msgf("IntrospectionUrl returned %d new packages", newRpms)

	return nil
}

func NewIntrospectHandler(db *gorm.DB) *IntrospectHandler {
	return &IntrospectHandler{
		Tx: db,
	}
}
