package notifications

import (
	"context"
	"strings"
	"time"

	"github.com/RedHatInsights/event-schemas-go/apps/repositories/v1"
	"github.com/Shopify/sarama"
	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/google/uuid"

	"github.com/cloudevents/sdk-go/protocol/kafka_sarama/v2"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/rs/zerolog/log"
)

func SendNotification(orgId string, eventName EventName, repos []repositories.Repositories) {
	kafkaServers := strings.Split(config.Get().Kafka.Bootstrap.Servers, ",")
	if len(kafkaServers) > 0 {
		eventNameStr := eventName.String()
		saramaConfig := sarama.NewConfig()

		saramaConfig.Version = sarama.V2_0_0_0
		saramaConfig.Consumer.Offsets.Initial = sarama.OffsetOldest
		// With NewProtocol you can use the same client both to send and receive.
		protocol, err := kafka_sarama.NewProtocol(kafkaServers, saramaConfig, "platform.notifications.ingress", "platform.notifications.ingress")
		if err != nil {
			log.Error().Msgf("failed to create kafka_sarama protocol: %s", err.Error())
		}

		c, err := cloudevents.NewClient(protocol, cloudevents.WithTimeNow(), cloudevents.WithUUIDs())
		if err != nil {
			log.Error().Msgf("failed to create cloudevents client, %v", err)
		}
		newUUID, _ := uuid.NewRandom()
		e := cloudevents.NewEvent()
		e.SetSource("urn:redhat:source:console:app:repositories")
		e.SetID(newUUID.String())
		e.SetType("com.redhat.console.repositories." + eventNameStr)
		e.SetSubject("urn:redhat:subject:console:rhel:" + eventNameStr)
		e.SetTime(time.Now())
		e.SetExtension("redhatorgid", orgId)
		e.SetExtension("redhatconsolebundle", "rhel")
		e.SetDataSchema("https://console.redhat.com/api/schemas/apps/repositories/v1/repository_events.json")

		data := repositories.RepositoryEvents{Repositories: repos}
		err = e.SetData(cloudevents.ApplicationJSON, data)

		if err != nil {
			log.Error().Msgf("failed to create cloudevents client, %v", err)
		}

		ctx := cloudevents.WithEncodingStructured(context.Background())

		// Send the event
		if result := c.Send(ctx, e); cloudevents.IsUndelivered(result) {
			log.Printf("failed to send: %v", result)
		} else {
			log.Printf("accepted: %t", cloudevents.IsACK(result))
		}
	}
}

func MapRepositoryResponse(importedRepo api.RepositoryResponse) repositories.Repositories {
	packageCount := int64(importedRepo.PackageCount)
	failedIntrospectionsCount := int64(importedRepo.FailedIntrospectionsCount)

	return repositories.Repositories{
		Name:                         importedRepo.Name,
		DistributionArch:             &importedRepo.DistributionArch,
		DistributionVersions:         importedRepo.DistributionVersions,
		FailedIntrospectionsCount:    &failedIntrospectionsCount,
		GPGKey:                       &importedRepo.GpgKey,
		LastIntrospectionError:       &importedRepo.LastIntrospectionError,
		LastIntrospectionTime:        &importedRepo.LastIntrospectionTime,
		LastSuccessIntrospectionTime: &importedRepo.LastIntrospectionSuccessTime,
		LastUpdateIntrospectionTime:  &importedRepo.LastIntrospectionUpdateTime,
		MetadataVerification:         &importedRepo.MetadataVerification,
		PackageCount:                 &packageCount,
		Status:                       &importedRepo.Status,
		URL:                          importedRepo.URL,
		UUID:                         importedRepo.UUID,
	}
}
