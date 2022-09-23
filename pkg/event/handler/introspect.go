package handler

import (
	"fmt"

	"github.com/content-services/content-sources-backend/pkg/event/message"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

type IntrospectHandler struct {
	Tx *gorm.DB
}

func (h *IntrospectHandler) dumpMessage(msg *message.IntrospectRequestMessage) {
	if msg == nil {
		return
	}
	log.Debug().Msgf("msg: %v", msg.State)
}

func (h *IntrospectHandler) OnMessage(msg *kafka.Message) error {
	var payload *message.IntrospectRequestMessage
	payload = &message.IntrospectRequestMessage{}
	if err := payload.UnmarshalJSON(msg.Value); err != nil {
		return fmt.Errorf("[IntrospectHandler.OnMessage] Error deserializing payload: %w", err)
	}
	var key string
	key = string(msg.Key)
	log.Debug().Msgf("OnMessage was called; Key=%s", key)
	// h.dumpMessage(payload)
	return nil
}

func NewIntrospectHandler(db *gorm.DB) *IntrospectHandler {
	return &IntrospectHandler{
		Tx: db,
	}
}
