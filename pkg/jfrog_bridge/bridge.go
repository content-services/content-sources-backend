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

// partnerConsumer is an enabled partner ready to consume the bridge topic.
type partnerConsumer struct {
	name            string
	consumerGroupID string
	handler         *BridgeHandler
}

// Start launches one Kafka consumer per enabled partner. Each partner uses
// its own consumer group, so a failure for one partner never commits the
// offset of another.
func Start(ctx context.Context, wg *sync.WaitGroup) {
	consumers := enabledPartnerConsumers(getSharedMetrics())
	if len(consumers) == 0 {
		log.Info().Msg("content-bridge: no partner is enabled")
		return
	}

	topic := config.Get().Options.LightwellBridgeTopic
	if kafka.TopicTranslationConfig != nil {
		topic = kafka.TopicTranslationConfig.GetReal(topic)
	}

	for _, c := range consumers {
		wg.Add(1)
		go func(c partnerConsumer) {
			defer wg.Done()
			runConsumerLoop(ctx, c, topic)
		}(c)
		log.Info().Str("partner", c.name).Str("topic", topic).Str("group", c.consumerGroupID).Msg("content-bridge partner started")
	}
}

// enabledPartnerConsumers builds a consumer for every enabled partner whose
// handler can be created. Disabled partners never open a consumer.
func enabledPartnerConsumers(metrics *bridgeMetrics) []partnerConsumer {
	var consumers []partnerConsumer
	for _, p := range partners {
		if !p.enabled() {
			log.Info().Str("partner", p.name).Msg("content-bridge partner is disabled")
			continue
		}
		handler, err := p.newHandler()
		if err != nil {
			log.Error().Err(err).Str("partner", p.name).Msg("content-bridge partner not started")
			continue
		}
		consumers = append(consumers, partnerConsumer{
			name:            p.name,
			consumerGroupID: p.consumerGroupID(),
			handler:         NewBridgeHandler(handler, metrics),
		})
	}
	return consumers
}

func runConsumerLoop(ctx context.Context, c partnerConsumer, topic string) {
	logger := log.With().Str("partner", c.name).Logger()

	saramaConfig, err := config.GetSaramaConfig()
	if err != nil {
		logger.Error().Err(err).Msg("content-bridge: failed to get sarama config")
		return
	}

	saramaConfig.Consumer.Offsets.Initial = sarama.OffsetNewest
	saramaConfig.Consumer.Return.Errors = true

	brokers := strings.Split(config.Get().Kafka.Bootstrap.Servers, ",")
	if len(brokers) == 0 || brokers[0] == "" {
		logger.Warn().Msg("content-bridge: no kafka brokers configured")
		return
	}

	client, err := sarama.NewConsumerGroup(brokers, c.consumerGroupID, saramaConfig)
	if err != nil {
		logger.Error().Err(err).Msg("content-bridge: failed to create consumer group")
		return
	}
	defer func() {
		if err := client.Close(); err != nil {
			logger.Error().Err(err).Msg("content-bridge: error closing consumer group")
		}
	}()

	go func() {
		for err := range client.Errors() {
			logger.Error().Err(err).Msg("content-bridge: consumer error")
		}
	}()

	for {
		if err := client.Consume(ctx, []string{topic}, c.handler); err != nil {
			logger.Error().Err(err).Msg("content-bridge: consume error")
		}
		if ctx.Err() != nil {
			logger.Info().Msg("content-bridge: context cancelled, stopping")
			return
		}
	}
}
