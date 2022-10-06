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

// TODO Bad practice but it needs more time to think a better
// isolated solution; probably defining Producer and Consumer
// structs with their methods, so the translation can be
// injected to them.
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

// GetInternal translate the name of a real topic to
// the internal topic name. This will be used by the
// consumers.
func (tm *TopicTranslation) GetInternal(realTopic string) string {
	if val, ok := tm.realToInternal[realTopic]; ok {
		return val
	}
	return ""
}

// GetReal translate the name of an internal topic
// to the real topic name. This will be used by the
// producers.
// Returns empty string when the topic is not found
// into the translation map.
func (tm *TopicTranslation) GetReal(internalTopic string) string {
	if val, ok := tm.internalToReal[internalTopic]; ok {
		return val
	}
	return ""
}
