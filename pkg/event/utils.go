package event

import (
	"fmt"
	"strings"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/rs/zerolog/log"
)

func getHeaderString(headers []kafka.Header) string {
	var output []string = make([]string, len(headers))
	for i, header := range headers {
		output[i] = fmt.Sprintf("%s: %s", header.Key, string(header.Value))
	}
	return fmt.Sprintf("{%s}", strings.Join(output, ", "))
}

func logEventMessageInfo(msg *kafka.Message, text string) {
	if msg == nil || text == "" {
		return
	}
	log.Info().
		Str("Topic", *msg.TopicPartition.Topic).
		Str("Key", string(msg.Key)).
		Str("Headers", getHeaderString(msg.Headers)).
		Msg(text)
}

func logEventMessageError(msg *kafka.Message, err error) {
	if msg == nil || err == nil {
		return
	}
	log.Error().
		Msgf("error processing event message: headers=%v; payload=%v: %s", msg.Headers, string(msg.Value), err.Error())
}
