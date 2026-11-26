package kafka

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/IBM/sarama"
	"github.com/content-services/content-sources-backend/pkg/utils"
	"github.com/rs/zerolog/log"
)

// Consumer is a thin wrapper around a sarama consumer group
type Consumer struct {
	group        sarama.ConsumerGroup
	retryBackoff time.Duration
}

// NewKafkaConsumer creates a consumer group client. cfg should come from NewSaramaConsumerConfig().
func NewKafkaConsumer(brokers []string, groupID string, cfg *sarama.Config) (*Consumer, error) {
	if len(brokers) == 0 {
		return nil, fmt.Errorf("kafka consumer (group %s): no brokers configured", groupID)
	}
	cfg.Consumer.Return.Errors = true
	group, err := sarama.NewConsumerGroup(brokers, groupID, cfg)
	if err != nil {
		return nil, fmt.Errorf("kafka consumer (group %s): %w", groupID, err)
	}
	return &Consumer{group: group, retryBackoff: cfg.Consumer.Group.Rebalance.Retry.Backoff}, nil
}

// Run consumes topics with the handler until ctx is canceled. It blocks, so run it in a goroutine.
func (c *Consumer) Run(ctx context.Context, topics []string, handler sarama.ConsumerGroupHandler) {
	topics = clowderTopicNames(topics)
	go func() {
		for err := range c.group.Errors() {
			log.Error().Err(err).Strs("topics", topics).Msg("kafka consumer error")
		}
	}()
	for {
		// Consume returns on rebalance, so it needs to be called in a loop
		err := c.group.Consume(ctx, topics, handler)
		if ctx.Err() != nil || errors.Is(err, sarama.ErrClosedConsumerGroup) {
			return
		}
		if err != nil {
			log.Warn().Err(err).Strs("topics", topics).Msg("error consuming, retrying")
			// avoid a hot loop of retries while brokers are unavailable
			if utils.SleepWithCancel(ctx, c.retryBackoff) != nil {
				return
			}
		}
	}
}

func (c *Consumer) Close() error {
	return c.group.Close()
}

func clowderTopicNames(topics []string) []string {
	if TopicTranslationConfig == nil {
		return topics
	}
	resolvedTopics := make([]string, len(topics))
	for i, t := range topics {
		resolvedTopics[i] = TopicTranslationConfig.GetReal(t)
	}
	return resolvedTopics
}
