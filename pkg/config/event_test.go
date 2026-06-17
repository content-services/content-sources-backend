package config

import (
	"os"
	"testing"
	"time"

	"github.com/IBM/sarama"
	"github.com/content-services/content-sources-backend/pkg/kafka"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadEnv(t *testing.T) {
	const existingKey = "EXISTING_KEY"
	const existingValue = "existing"
	const unexistingKey = "UNEXISTING_KEY"
	const defaultValue = "default"
	type TestCaseGiven struct {
		Key          string
		DefaultValue string
	}
	type TestCaseExpected string
	type TestCase struct {
		Given    TestCaseGiven
		Expected TestCaseExpected
	}

	var testCases = []TestCase{
		{
			Given: TestCaseGiven{
				Key:          existingKey,
				DefaultValue: defaultValue,
			},
			Expected: existingValue,
		},
		{
			Given: TestCaseGiven{
				Key:          unexistingKey,
				DefaultValue: defaultValue,
			},
			Expected: defaultValue,
		},
	}

	os.Unsetenv(unexistingKey)
	os.Setenv(existingKey, existingValue)

	for _, testCase := range testCases {
		result := readEnv(testCase.Given.Key, testCase.Given.DefaultValue)
		assert.Equal(t, string(testCase.Expected), result)
	}
}

func TestGetSaramaConfigProducerSettings(t *testing.T) {
	LoadedConfig.Kafka = kafka.KafkaConfig{}
	LoadedConfig.Kafka.Message.Send.Max.Retries = 15
	LoadedConfig.Kafka.Retry.Backoff.Ms = 100
	LoadedConfig.Kafka.Request.Required.Acks = -1

	saramaConfig, err := GetSaramaConfig()
	require.NoError(t, err)

	assert.Equal(t, 15, saramaConfig.Producer.Retry.Max)
	assert.Equal(t, 100*time.Millisecond, saramaConfig.Producer.Retry.Backoff)
	assert.Equal(t, sarama.WaitForAll, saramaConfig.Producer.RequiredAcks)
}

func TestProducerRequiredAcks(t *testing.T) {
	assert.Equal(t, sarama.NoResponse, producerRequiredAcks(0))
	assert.Equal(t, sarama.WaitForLocal, producerRequiredAcks(1))
	assert.Equal(t, sarama.WaitForAll, producerRequiredAcks(-1))
	assert.Equal(t, sarama.WaitForAll, producerRequiredAcks(99))
}
