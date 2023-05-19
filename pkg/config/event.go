package config

import (
	"os"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/event"
	"github.com/content-services/content-sources-backend/pkg/event/schema"
	clowder "github.com/redhatinsights/app-common-go/pkg/api/v1"
	"github.com/spf13/viper"
)

func addEventConfigDefaults(options *viper.Viper) {
	options.SetDefault("kafka.timeout", 10000)
	options.SetDefault("kafka.group.id", "content-sources")
	options.SetDefault("kafka.auto.offset.reset", "latest")
	options.SetDefault("kafka.auto.commit.interval.ms", 5000)
	options.SetDefault("kafka.request.required.acks", -1) // -1 == "all"
	options.SetDefault("kafka.message.send.max.retries", 15)
	options.SetDefault("kafka.retry.backoff.ms", 100)
	if clowder.IsClowderEnabled() {
		cfg := clowder.LoadedConfig
		event.TopicTranslationConfig = event.NewTopicTranslationWithClowder(cfg)
		options.SetDefault("kafka.bootstrap.servers", strings.Join(clowder.KafkaServers, ","))

		// Prepare topics
		topics := []string{}
		for _, value := range clowder.KafkaTopics {
			if strings.Contains(value.Name, "content-sources") {
				topics = append(topics, value.Name)
			}
		}
		options.SetDefault("kafka.topics", strings.Join(topics, ","))

		if cfg != nil && cfg.Kafka != nil && cfg.Kafka.Brokers != nil && len(cfg.Kafka.Brokers) > 0 {
			if cfg.Kafka.Brokers[0].Cacert != nil {
				// This method is writing only the first CA but if
				// that behavior changes in the future, nothing will
				// be changed here
				caPath, err := cfg.KafkaCa(cfg.Kafka.Brokers...)
				if err != nil {
					panic("Kafka CA failed to write")
				}
				options.Set("kafka.capath", caPath)
			}

			broker := cfg.Kafka.Brokers[0]
			if broker.Authtype != nil {
				options.Set("kafka.sasl.username", *broker.Sasl.Username)
				options.Set("kafka.sasl.password", *broker.Sasl.Password)
				options.Set("kafka.sasl.mechanism", *broker.Sasl.SaslMechanism)
				if broker.Sasl.SecurityProtocol != nil { // nolint:staticcheck
					options.Set("kafka.sasl.protocol", *broker.Sasl.SecurityProtocol) // nolint:staticcheck
				}
				if broker.SecurityProtocol != nil {
					options.Set("kafka.sasl.protocol", *broker.SecurityProtocol)
				}
			}
		}
	} else {
		// If clowder is not present, set defaults to local configuration
		event.TopicTranslationConfig = event.NewTopicTranslationWithDefaults()
		options.SetDefault("kafka.bootstrap.servers", readEnv("KAFKA_BOOTSTRAP_SERVERS", ""))
		options.SetDefault("kafka.topics", schema.TopicIntrospect)
	}
}

func readEnv(key string, def string) string {
	value, ok := os.LookupEnv(key)
	if !ok {
		value = def
	}
	return value
}
