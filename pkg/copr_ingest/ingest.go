package copr_ingest

import (
	"context"
	"sync"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/instrumentation"
	"github.com/content-services/content-sources-backend/pkg/kafka"
	"github.com/content-services/content-sources-backend/pkg/tasks/client"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	"github.com/rs/zerolog/log"
)

// Start runs the Copr build event consumer in a goroutine tracked by wg until ctx is canceled.
// If setup fails, it logs the error and returns without starting the consumer.
func Start(ctx context.Context, wg *sync.WaitGroup, metrics *instrumentation.Metrics) {
	cfg := config.Get()
	saramaConfig, err := kafka.NewSaramaConsumerConfig(cfg.Kafka)
	if err != nil {
		log.Error().Err(err).Msg("copr-ingest: error getting sarama config")
		return
	}
	consumer, err := kafka.NewKafkaConsumer(cfg.Kafka.Brokers(), cfg.CoprIngest.ConsumerGroupID, saramaConfig)
	if err != nil {
		log.Error().Err(err).Msg("copr-ingest: failed to create kafka consumer")
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

func closeConsumer(consumer *kafka.Consumer) {
	if err := consumer.Close(); err != nil {
		log.Error().Err(err).Msg("copr-ingest: error closing kafka consumer")
	}
}
