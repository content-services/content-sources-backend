package kafka

import (
	"crypto/sha512"
	"fmt"
	"strings"
	"time"

	"github.com/IBM/sarama"
	tlsutil "github.com/RedHatInsights/insights-operator-utils/tls"
)

// NewSaramaConfig returns a new sarama config with the settings shared by producers and consumers,
// such as the protocol version, TLS, and SASL.
func NewSaramaConfig(c KafkaConfig) (*sarama.Config, error) {
	saramaConfig := sarama.NewConfig()

	saramaConfig.Version = sarama.V2_0_0_0

	if strings.Contains(c.Sasl.Protocol, "SSL") {
		saramaConfig.Net.TLS.Enable = true
	}

	if c.Capath != "" {
		tlsConfig, err := tlsutil.NewTLSConfig(c.Capath)
		if err != nil {
			return nil, fmt.Errorf("unable to load TLS config for %s cert: %w", c.Capath, err)
		}
		saramaConfig.Net.TLS.Config = tlsConfig
	}

	if strings.HasPrefix(c.Sasl.Protocol, "SASL_") {
		saramaConfig.Net.SASL.Enable = true
		saramaConfig.Net.SASL.User = c.Sasl.Username
		saramaConfig.Net.SASL.Password = c.Sasl.Password
		saramaConfig.Net.SASL.Mechanism = sarama.SASLMechanism(c.Sasl.Mechanism)
		if saramaConfig.Net.SASL.Mechanism == sarama.SASLTypeSCRAMSHA512 {
			saramaConfig.Net.SASL.Handshake = true
			saramaConfig.Net.SASL.SCRAMClientGeneratorFunc = func() sarama.SCRAMClient {
				return &SCRAMClient{HashGeneratorFcn: sha512.New}
			}
		}
	}
	return saramaConfig, nil
}

// NewSaramaConsumerConfig returns a new sarama config with the shared settings and the consumer group
// settings applied. Callers can override any setting on the returned config before using it.
func NewSaramaConsumerConfig(c KafkaConfig) (*sarama.Config, error) {
	saramaConfig, err := NewSaramaConfig(c)
	if err != nil {
		return nil, err
	}

	// Only applies when a consumer group has no committed offset
	switch c.Auto.Offset.Reset {
	case "", "latest":
		saramaConfig.Consumer.Offsets.Initial = sarama.OffsetNewest
	case "earliest":
		saramaConfig.Consumer.Offsets.Initial = sarama.OffsetOldest
	default:
		return nil, fmt.Errorf("invalid kafka.auto.offset.reset %q, expected \"latest\" or \"earliest\"", c.Auto.Offset.Reset)
	}

	if backoffMs := c.Consumer.Retry.Backoff.Ms; backoffMs > 0 {
		saramaConfig.Consumer.Group.Rebalance.Retry.Backoff = time.Duration(backoffMs) * time.Millisecond
	}
	return saramaConfig, nil
}
