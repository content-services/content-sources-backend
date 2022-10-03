package event

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

// type KafkaConfig struct {
// 	Timeout              int
// 	GroupId              string
// 	AutoOffsetReset      string
// 	AutoCommitIntervalMs int
// 	BootstrapServers     string
// 	Topics               []string

// 	SaslUsername  string
// 	SaslPassword  string
// 	SaslMechanism string
// 	SaslProtocol  string

// 	RequestTimeoutMs    int
// 	RequestRequiredAcks int

// 	Capath                string
// 	MessageSendMaxRetries int
// 	RetryBackoffMs        int
// }
