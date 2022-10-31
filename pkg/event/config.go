package event

// The kafka configuration that is used for produce and
// consume messages
// Related with configuration that is used by the librdkafka
// library.
// See: https://github.com/edenhill/librdkafka/blob/master/CONFIGURATION.md
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
}
