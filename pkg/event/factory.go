package event

import (
	"strings"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/rs/zerolog/log"
)

// Adapted from: https://github.com/RedHatInsights/playbook-dispatcher/blob/master/internal/response-consumer/main.go#L21

func Start(config *config.Configuration, handler Eventable) {
	var (
		err      error
		consumer *kafka.Consumer
	)

	dumpKafkaConfiguration(config)
	if consumer, err = NewConsumer(config); err != nil {
		log.Logger.Panic().Msgf("error creating consumer: %s", err.Error())
		return
	}
	defer consumer.Close()

	start := NewConsumerEventLoop(consumer, handler)
	start()
}

// type KafkaConfig struct {
// 	Timeout int
// 	Group   struct {
// 		Id string
// 	}
// 	Auto struct {
// 		Offset struct {
// 			Reset string
// 		}
// 		Commit struct {
// 			Interval struct {
// 				Ms int
// 			}
// 		}
// 	}
// 	Bootstrap struct {
// 		Servers string
// 	}
// 	Topics []string
// 	Sasl   struct {
// 		Username string
// 		Password string
// 		Mechnism string
// 		Protocol string
// 	}
// 	Request struct {
// 		Timeout struct {
// 			Ms int
// 		}
// 		Required struct {
// 			Acks int
// 		}
// 	}
// 	Capath  string
// 	Message struct {
// 		Send struct {
// 			Max struct {
// 				Retries int
// 			}
// 		}
// 	}
// 	Retry struct {
// 		Backoff struct {
// 			Ms int
// 		}
// 	}
// }
func dumpKafkaConfiguration(config *config.Configuration) {
	log.Logger.Debug().Msg("BEGIN dumpKafkaConfiguration")
	log.Logger.Debug().Msgf("Timeout: %d", config.Kafka.Timeout)
	log.Logger.Debug().Msgf("Group.Id: %s", config.Kafka.Group.Id)
	log.Logger.Debug().Msgf("Auto.Offset.Reset: %s", config.Kafka.Auto.Offset.Reset)
	log.Logger.Debug().Msgf("Auto.Commit.Interval.Ms: %d", config.Kafka.Auto.Commit.Interval.Ms)
	log.Logger.Debug().Msgf("Bootstrap.Servers: %s", config.Kafka.Bootstrap.Servers)
	log.Logger.Debug().Msgf("Topics: %s", strings.Join(config.Kafka.Topics, ", "))
	log.Logger.Debug().Msgf("Sasl.Username: %s", config.Kafka.Sasl.Username)
	log.Logger.Debug().Msgf("Sasl.Password: %s", config.Kafka.Sasl.Password)
	log.Logger.Debug().Msgf("Sasl.Mechnism: %s", config.Kafka.Sasl.Mechnism)
	log.Logger.Debug().Msgf("Sasl.Protocol: %s", config.Kafka.Sasl.Protocol)
	log.Logger.Debug().Msgf("Request.Timeout.Ms: %d", config.Kafka.Request.Timeout.Ms)
	log.Logger.Debug().Msgf("Request.Required.Acks: %d", config.Kafka.Request.Required.Acks)
	log.Logger.Debug().Msgf("Capath: %s", config.Kafka.Capath)
	log.Logger.Debug().Msgf("Message.Send.Max.Retries: %d", config.Kafka.Message.Send.Max.Retries)
	log.Logger.Debug().Msgf("Retry.Backoff.Ms: %d", config.Kafka.Retry.Backoff.Ms)
	log.Logger.Debug().Msg("END dumpKafkaConfiguration")
}
