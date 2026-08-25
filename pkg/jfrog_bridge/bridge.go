package jfrog_bridge

import (
	"context"
	"strings"
	"sync"

	"github.com/IBM/sarama"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/kafka"
	"github.com/rs/zerolog/log"
)

// Start launches the JFrog bridge Kafka consumer if enabled.
func Start(ctx context.Context, wg *sync.WaitGroup) {
	cfg := loadConfig()
	if !cfg.Enabled {
		log.Info().Msg("jfrog-bridge is disabled")
		return
	}

	metrics := getSharedMetrics()

	registry := newRegistryClient(cfg)
	jfrog := newJFrogClient(cfg)
	evidence, err := newEvidenceCreator(cfg)
	if err != nil {
		log.Error().Err(err).Msg("failed to create evidence creator")
		return
	}

	handler := NewBridgeHandler(registry, jfrog, evidence, metrics)
	handler.registryURL = cfg.RegistryURL

	topic := config.Get().Options.LightwellBridgeTopic
	if kafka.TopicTranslationConfig != nil {
		topic = kafka.TopicTranslationConfig.GetReal(topic)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		runConsumerLoop(ctx, cfg, handler, topic)
	}()

	log.Info().Str("topic", topic).Str("group", cfg.ConsumerGroupID).Msg("jfrog-bridge started")
}

func runConsumerLoop(ctx context.Context, cfg bridgeConfig, handler *BridgeHandler, topic string) {
	saramaConfig, err := config.GetSaramaConfig()
	if err != nil {
		log.Error().Err(err).Msg("jfrog-bridge: failed to get sarama config")
		return
	}

	saramaConfig.Consumer.Offsets.Initial = sarama.OffsetNewest
	saramaConfig.Consumer.Return.Errors = true

	brokers := strings.Split(config.Get().Kafka.Bootstrap.Servers, ",")
	if len(brokers) == 0 || brokers[0] == "" {
		log.Warn().Msg("jfrog-bridge: no kafka brokers configured")
		return
	}

	client, err := sarama.NewConsumerGroup(brokers, cfg.ConsumerGroupID, saramaConfig)
	if err != nil {
		log.Error().Err(err).Msg("jfrog-bridge: failed to create consumer group")
		return
	}
	defer func() {
		if err := client.Close(); err != nil {
			log.Error().Err(err).Msg("jfrog-bridge: error closing consumer group")
		}
	}()

	go func() {
		for err := range client.Errors() {
			log.Error().Err(err).Msg("jfrog-bridge: consumer error")
		}
	}()

	for {
		if err := client.Consume(ctx, []string{topic}, handler); err != nil {
			log.Error().Err(err).Msg("jfrog-bridge: consume error")
		}
		if ctx.Err() != nil {
			log.Info().Msg("jfrog-bridge: context cancelled, stopping")
			return
		}
	}
}
