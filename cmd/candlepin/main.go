package main

import (
	"context"
	"fmt"
	"os"

	"github.com/content-services/content-sources-backend/pkg/clients/candlepin_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

func main() {
	config.Load()
	config.ConfigureLogging()
	err := db.Connect()
	if err != nil {
		log.Logger.Error().Err(err).Msg("Could not config db")
	}

	args := os.Args
	usage := "arguments:  ./candlepin init | list-contents | add-system CONSUMER_UUID TEMPLATE_NAME"
	if len(args) < 2 {
		log.Fatal().Msg(usage)
	}
	client := candlepin_client.NewCandlepinClient()
	switch args[1] {
	case "init":
		initCandlepin(client)
	case "list-contents":
		listContents(client)
	case "add-system":
		addSystem(db.DB, client, args[2], args[3])
	default:
		log.Fatal().Msg(usage)
	}
}

func addSystem(db *gorm.DB, client candlepin_client.CandlepinClient, consumerUuid string, templateName string) {
	ctx := context.Background()
	dao := dao.GetDaoRegistry(db)

	template, err := dao.Template.InternalOnlyFetchByName(ctx, templateName)
	if err != nil {
		log.Fatal().Err(err).Msgf("Failed to find template with name %v", templateName)
	}
	err = client.AssociateEnvironment(ctx, candlepin_client.DevelOrgKey, template.UUID, consumerUuid)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to associate system to env")
	}
}

func initCandlepin(client candlepin_client.CandlepinClient) {
	ctx := context.Background()
	err := client.CreateOwner(ctx)
	if err != nil {
		log.Logger.Fatal().Err(err).Msg("Could not create org")
	}

	err = client.ImportManifest(ctx, "./configs/manifest.zip")
	if err != nil {
		log.Logger.Fatal().Err(err).Msg("Could not import manifest")
	}
}

func listContents(client candlepin_client.CandlepinClient) {
	contents, _, err := client.ListContents(context.Background(), candlepin_client.DevelOrgKey)
	if err != nil {
		log.Logger.Fatal().Err(err).Msg("Could not list contents")
	}
	for _, label := range contents {
		fmt.Print(label, "\n")
	}
}
