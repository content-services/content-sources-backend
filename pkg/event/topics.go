package event

import (
	"github.com/content-services/content-sources-backend/pkg/event/schema"
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

// NewDefaultTopicMap Build a default topic map that map
// all the allowed topics to itselfs
// Return A TopicMap initialized as default values
func NewTopicTranslationWithDefaults() *TopicTranslation {
	var tm *TopicTranslation = &TopicTranslation{
		internalToReal: make(map[string]string),
		realToInternal: make(map[string]string),
	}
	for _, topic := range schema.AllowedTopics {
		tm.internalToReal[topic] = topic
		tm.realToInternal[topic] = topic
	}
	return tm
}

// NewTopicTranslationWithClowder Build a topic map based into the
// clowder configuration.
func NewTopicTranslationWithClowder(cfg *clowder.AppConfig) *TopicTranslation {
	if cfg == nil {
		return NewTopicTranslationWithDefaults()
	}

	var tm *TopicTranslation = &TopicTranslation{
		internalToReal: make(map[string]string),
		realToInternal: make(map[string]string),
	}
	for _, topic := range cfg.Kafka.Topics {
		tm.internalToReal[topic.RequestedName] = topic.Name
		tm.realToInternal[topic.Name] = topic.RequestedName
		log.Debug().Str(topic.RequestedName, topic.Name).Msg("internalToReal")
		log.Debug().Str(topic.Name, topic.RequestedName).Msg("realToInternal")
	}
	return tm
}

// GetInternal translates the topic's "Name" to the "RequestedName".
// This will be used by consumers.
// Returns an empty string when the topic is not found
// Example:
// "name": "platform-tmp-12345",
// "requestedName": "platform.notifications.ingress"
func (tm *TopicTranslation) GetInternal(realTopic string) string {
	if val, ok := tm.realToInternal[realTopic]; ok {
		return val
	}
	return ""
}

// GetReal translates the topic's "RequestedName" to the "Name".
// This will be used by producers.
// Returns an empty string when the topic is not found.
// Example:
// "name": "platform-tmp-12345",
// "requestedName": "platform.notifications.ingress"
func (tm *TopicTranslation) GetReal(internalTopic string) string {
	if val, ok := tm.internalToReal[internalTopic]; ok {
		return val
	}
	return ""
}
