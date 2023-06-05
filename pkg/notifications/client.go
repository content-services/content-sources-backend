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
		log.Warn().Msgf("Notification started: %v", config.Get().NotificationsClient)
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
		e.SetDataSchema("https://console.redhat.com/api/schemas/apps/repositories/v1/repository-events.json")

		data := repositories.RepositoryEvents{Repositories: repos}
		err := e.SetData(cloudevents.ApplicationJSON, data)

		if err != nil {
			log.Error().Err(err).Msg("failed to create cloudevents client")
			return
		}

		ctx := cloudevents.WithEncodingStructured(context.Background())
		// Send the event
		if result := config.Get().NotificationsClient.Send(ctx, e); cloudevents.IsUndelivered(result) {
			log.Error().Msgf("Notification message failed to send: %v", result)
			return
		} else {
			log.Warn().Msgf("Notification message accepted: %t", cloudevents.IsACK(result))
		}
		ctx.Done()
	} else {
		log.Warn().Msgf("config.Get().NotificationsClient is null")
	}
}

func MapRepositoryResponse(importedRepo api.RepositoryResponse) repositories.Repositories {
	packageCount := int64(importedRepo.PackageCount)
	failedIntrospectionsCount := int64(importedRepo.FailedIntrospectionsCount)

	return repositories.Repositories{
		Name:                         importedRepo.Name,
		URL:                          importedRepo.URL,
		UUID:                         importedRepo.UUID,
		DistributionVersions:         importedRepo.DistributionVersions,
		FailedIntrospectionsCount:    &failedIntrospectionsCount,                   // Optional but defaults to 0
		PackageCount:                 &packageCount,                                // Optional but defaults to 0
		MetadataVerification:         &importedRepo.MetadataVerification,           // Optional but defaults to false
		DistributionArch:             SetEmptyToNil(importedRepo.DistributionArch), // Below are all optional, we need to set them as nil if empty for the cloud schema
		GPGKey:                       SetEmptyToNil(importedRepo.GpgKey),
		LastIntrospectionError:       SetEmptyToNil(importedRepo.LastIntrospectionError),
		LastIntrospectionTime:        SetEmptyToNil(importedRepo.LastIntrospectionTime),
		LastSuccessIntrospectionTime: SetEmptyToNil(importedRepo.LastIntrospectionSuccessTime),
		LastUpdateIntrospectionTime:  SetEmptyToNil(importedRepo.LastIntrospectionUpdateTime),
		Status:                       SetEmptyToNil(importedRepo.Status),
	}
}

func SetEmptyToNil(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
