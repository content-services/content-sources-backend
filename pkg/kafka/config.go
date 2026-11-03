package kafka

import "strings"

// KafkaConfig holds the options for our Sarama-based Kafka clients.
// Key names come from librdkafka (https://github.com/edenhill/librdkafka/blob/master/CONFIGURATION.md),
// but Sarama doesn't read them: a field only works if our code passes it to Sarama
// (e.g., NewSaramaConsumerConfig). Unused fields, like Retry.Backoff.Ms, do nothing.
type KafkaConfig struct {
	Timeout int
	Group   struct {
		Id string
	}
	Auto struct {
		Offset struct {
			Reset string
		}
		Commit struct {
			Interval struct {
				Ms int
			}
		}
	}
	Bootstrap struct {
		Servers string
	}
	Topics []string
	Sasl   struct {
		Username  string
		Password  string
		Mechanism string
		Protocol  string
	}
	Request struct {
		Timeout struct {
			Ms int
		}
		Required struct {
			Acks int
		}
	}
	Capath  string
	Message struct {
		Send struct {
			Max struct {
				Retries int
			}
		}
	}
	Retry struct {
		Backoff struct {
			Ms int
		}
	}
	Consumer struct {
		Retry struct {
			Backoff struct {
				Ms int
			}
		}
	}
}

// Brokers returns the configured bootstrap servers, ignoring empty entries
func (c KafkaConfig) Brokers() []string {
	var brokers []string
	for broker := range strings.SplitSeq(c.Bootstrap.Servers, ",") {
		if broker = strings.TrimSpace(broker); broker != "" {
			brokers = append(brokers, broker)
		}
	}
	return brokers
}
