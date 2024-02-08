package main

import (
	"context"
	"fmt"
	"os"

	"github.com/content-services/content-sources-backend/pkg/candlepin_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/rs/zerolog/log"
)

func main() {
	config.Load()
	config.ConfigureLogging()
	err := db.Connect()
	if err != nil {
		log.Logger.Error().Err(err).Msg("Could not config db")
	}

	args := os.Args
	usage := "arguments:  ./candlepin init | list-contents"
	if len(args) < 2 {
		log.Fatal().Msg(usage)
	}
	client, err := candlepin_client.NewCandlepinClient(context.Background())
	if err != nil {
		log.Logger.Error().Err(err).Msg("Could not config candlepin")
	}
	if args[1] == "init" {
		initCandlepin(client)
	} else if args[1] == "list-contents" {
		listContents(client)
	} else {
		log.Fatal().Msg(usage)
	}
}

func initCandlepin(client candlepin_client.CandlepinClient) {
	err := client.CreateOwner()
	if err != nil {
		log.Logger.Fatal().Err(err).Msg("Could not create org")
	}

	err = client.ImportManifest("./configs/manifest.zip")
	if err != nil {
		log.Logger.Fatal().Err(err).Msg("Could not import manifest")
	}
}

func listContents(client candlepin_client.CandlepinClient) {
	contents, err := client.ListContents(candlepin_client.DevelOrgKey)
	if err != nil {
		log.Logger.Fatal().Err(err).Msg("Could not list contents")
	}
	for _, label := range contents {
		fmt.Print(label, "\n")
	}
}
