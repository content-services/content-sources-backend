package config

import (
	"crypto/sha512"
	"fmt"
	"os"
	"strings"

	"github.com/IBM/sarama"
	"github.com/RedHatInsights/insights-operator-utils/tls"
	"github.com/cloudevents/sdk-go/protocol/kafka_sarama/v2"
	"github.com/cloudevents/sdk-go/v2"
	"github.com/content-services/content-sources-backend/pkg/kafka"
	clowder "github.com/redhatinsights/app-common-go/pkg/api/v1"
	"github.com/rs/zerolog/log"
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
		kafka.TopicTranslationConfig = kafka.NewTopicTranslationWithClowder(cfg)
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
				if broker.SecurityProtocol != nil {
					options.Set("kafka.sasl.protocol", *broker.SecurityProtocol)
				}
			}
		}
	} else {
		// If clowder is not present, set defaults to local configuration
		kafka.TopicTranslationConfig = kafka.NewTopicTranslationWithClowder(nil)
		options.SetDefault("kafka.bootstrap.servers", readEnv("KAFKA_BOOTSTRAP_SERVERS", ""))
	}
}

func readEnv(key string, def string) string {
	value, ok := os.LookupEnv(key)
	if !ok {
		value = def
	}
	return value
}

// SetupCloudEventsKafkaClient create the cloud event kafka client that will send event to the given kafka topic
func SetupCloudEventsKafkaClient(topic string) (v2.Client, error) {
	kafkaServers := strings.Split(LoadedConfig.Kafka.Bootstrap.Servers, ",")
	saramaConfig, err := GetSaramaConfig()
	if err != nil {
		return nil, fmt.Errorf("error getting sarama config: %w", err)
	}

	topicTranslator := kafka.NewTopicTranslationWithClowder(clowder.LoadedConfig)
	mappedTopicName := topicTranslator.GetReal(topic)

	if mappedTopicName == "" {
		mappedTopicName = topic
	}

	protocol, err := kafka_sarama.NewSender(kafkaServers, saramaConfig, mappedTopicName)
	if err != nil {
		return nil, fmt.Errorf("failed to create kafka_sarama protocol: %w", err)
	}

	c, err := v2.NewClient(protocol, v2.WithTimeNow(), v2.WithUUIDs())
	if err != nil {
		return nil, fmt.Errorf("failed to create cloud event client: %w", err)
	}
	return c, nil
}

// SetupTemplateEvents creates the cloud event kafka client for sending event to the patch service
func SetupTemplateEvents() {
	if LoadedConfig.Options.TemplateEventTopic == "" {
		return
	}

	if len(LoadedConfig.Kafka.Bootstrap.Servers) == 0 {
		log.Warn().Msg("SetupTemplateEvents: clowder.KafkaServers and configured broker was empty")
		return
	}

	client, err := SetupCloudEventsKafkaClient(LoadedConfig.Options.TemplateEventTopic)
	if err != nil {
		log.Error().Err(err).Msg("SetupTemplateEvents failed")
		return
	}
	LoadedConfig.TemplateEventClient = client
}

// SetupNotifications creates the cloud event kafka client for sending event to the event service
func SetupNotifications() {
	if !LoadedConfig.Options.EnableNotifications {
		return
	}

	if len(LoadedConfig.Kafka.Bootstrap.Servers) == 0 {
		log.Warn().Msg("SetupNotifications: clowder.KafkaServers and configured broker was empty")
		return
	}

	client, err := SetupCloudEventsKafkaClient("platform.notifications.ingress")
	if err != nil {
		log.Error().Err(err).Msg("SetupNotifications failed")
		return
	}
	LoadedConfig.NotificationsClient = client
}

func GetSaramaConfig() (*sarama.Config, error) {
	saramaConfig := sarama.NewConfig()

	saramaConfig.Version = sarama.V2_0_0_0
	saramaConfig.Consumer.Offsets.Initial = sarama.OffsetOldest

	if strings.Contains(LoadedConfig.Kafka.Sasl.Protocol, "SSL") {
		saramaConfig.Net.TLS.Enable = true
	}

	if LoadedConfig.Kafka.Capath != "" {
		tlsConfig, err := tlsutil.NewTLSConfig(LoadedConfig.Kafka.Capath)
		if err != nil {
			return nil, fmt.Errorf("unable to load TLS config for %s cert: %w", LoadedConfig.Kafka.Capath, err)
		}
		saramaConfig.Net.TLS.Config = tlsConfig
	}

	if strings.HasPrefix(LoadedConfig.Kafka.Sasl.Protocol, "SASL_") {
		saramaConfig.Net.SASL.Enable = true
		saramaConfig.Net.SASL.User = LoadedConfig.Kafka.Sasl.Username
		saramaConfig.Net.SASL.Password = LoadedConfig.Kafka.Sasl.Password
		saramaConfig.Net.SASL.Mechanism = sarama.SASLMechanism(LoadedConfig.Kafka.Sasl.Mechanism)
		if saramaConfig.Net.SASL.Mechanism == sarama.SASLTypeSCRAMSHA512 {
			saramaConfig.Net.SASL.Handshake = true
			saramaConfig.Net.SASL.SCRAMClientGeneratorFunc = func() sarama.SCRAMClient {
				return &kafka.SCRAMClient{HashGeneratorFcn: sha512.New}
			}
		}
	}
	return saramaConfig, nil
}
