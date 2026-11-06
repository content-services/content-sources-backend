package copr_ingest

import (
	"context"
	"fmt"
	"sync"

	"github.com/IBM/sarama"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/instrumentation"
	"github.com/content-services/content-sources-backend/pkg/kafka"
	"github.com/content-services/content-sources-backend/pkg/tasks/client"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	"github.com/rs/zerolog/log"
)

// Start launches the Copr build event consumer; setup errors are only logged, so they don't stop the API.
func Start(ctx context.Context, wg *sync.WaitGroup, metrics *instrumentation.Metrics) {
	cfg := config.Get()
	consumer, err := newConsumer(cfg)
	if err != nil {
		log.Error().Err(err).Msg("copr-ingest: failed to initialize kafka consumer")
		return
	}
	pgQueue, err := queue.NewPgQueue(ctx, db.GetUrl())
	if err != nil {
		log.Error().Err(err).Msg("copr-ingest: failed to create task queue")
		closeConsumer(consumer)
		return
	}
	taskClient := client.NewTaskClient(&pgQueue)

	wg.Go(func() {
		defer func() {
			closeConsumer(consumer)
			pgQueue.Close()
		}()
		handler := NewBuildEndHandler(taskClient, metrics)
		log.Info().Str("topic", cfg.CoprIngest.Topic).Str("group", cfg.CoprIngest.ConsumerGroupID).Msg("copr-ingest started")
		consumer.Run(ctx, []string{cfg.CoprIngest.Topic}, handler)
	})
}

// newConsumer creates the Kafka consumer for the Copr topic.
func newConsumer(cfg *config.Configuration) (*kafka.Consumer, error) {
	saramaConfig, err := kafka.NewSaramaConsumerConfig(cfg.Kafka)
	if err != nil {
		return nil, fmt.Errorf("error getting sarama config: %w", err)
	}
	// Without a committed offset, don't skip events already published (duplicates are handled by the snapshot task)
	saramaConfig.Consumer.Offsets.Initial = sarama.OffsetOldest
	consumer, err := kafka.NewKafkaConsumer(cfg.Kafka.Brokers(), cfg.CoprIngest.ConsumerGroupID, saramaConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create kafka consumer: %w", err)
	}
	return consumer, nil
}

func closeConsumer(consumer *kafka.Consumer) {
	if err := consumer.Close(); err != nil {
		log.Error().Err(err).Msg("copr-ingest: error closing kafka consumer")
	}
}
