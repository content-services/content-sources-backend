package event

import (
	clowder "github.com/redhatinsights/app-common-go/pkg/api/v1"
	"github.com/rs/zerolog/log"
)

// TopicMap is used to map between real and internal topics, this is
// it could be that the name we indicate for the topics into the
// clowderapp resource be different from the real created in kafka,
// so this type allow to preproce the mappings, and use them when
// needed to translate them into the producer and consumer functions
type TopicTranslation struct {
	internalToReal map[string]string
	realToInternal map[string]string
}

// It store the mapping between the internal topic managed by
// the service and the real topic managed by kafka
var TopicTranslationConfig *TopicTranslation = nil

// NewTopicTranslationWithClowder Build a topic map based into the
// clowder configuration.
func NewTopicTranslationWithClowder(cfg *clowder.AppConfig) *TopicTranslation {
	tm := &TopicTranslation{
		internalToReal: make(map[string]string),
		realToInternal: make(map[string]string),
	}
	if cfg != nil {
		for _, topic := range cfg.Kafka.Topics {
			tm.internalToReal[topic.RequestedName] = topic.Name
			tm.realToInternal[topic.Name] = topic.RequestedName
			log.Debug().Str(topic.RequestedName, topic.Name).Msg("internalToReal")
			log.Debug().Str(topic.Name, topic.RequestedName).Msg("realToInternal")
		}
	}
	return tm
}

// GetInternal translates the topic's "Name" to the "RequestedName".
// This will be used by consumers.
// Returns input string when the topic is not found
// Example:
// "name": "platform-tmp-12345",
// "requestedName": "platform.notifications.ingress"
func (tm *TopicTranslation) GetInternal(realTopic string) string {
	if val, ok := tm.realToInternal[realTopic]; ok {
		return val
	}
	return realTopic
}

// GetReal translates the topic's "RequestedName" to the "Name".
// This will be used by producers.
// Returns  input string when the topic is not found.
// Example:
// "name": "platform-tmp-12345",
// "requestedName": "platform.notifications.ingress"
func (tm *TopicTranslation) GetReal(internalTopic string) string {
	if val, ok := tm.internalToReal[internalTopic]; ok {
		return val
	}
	return internalTopic
}
