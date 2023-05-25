package notifications

import (
	"context"
	"time"

	"github.com/RedHatInsights/event-schemas-go/apps/repositories/v1"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

func SendNotification(orgId string, eventName EventName, repos []repositories.Repositories) {
	if config.Get().NotificationsClient != nil {
		eventNameStr := eventName.String()
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
		err := e.SetData(cloudevents.ApplicationJSON, data)

		if err != nil {
			log.Error().Err(err).Msg("failed to create cloudevents client")
			return
		}

		ctx := cloudevents.WithEncodingStructured(context.Background())
		// Send the event
		if result := config.Get().NotificationsClient.Send(ctx, e); cloudevents.IsUndelivered(result) {
			log.Error().Err(err).Msg("Notification message failed to send")
			return
		} else {
			log.Debug().Msgf("Notification message accepted: %t", cloudevents.IsACK(result))
		}
		ctx.Done()
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
