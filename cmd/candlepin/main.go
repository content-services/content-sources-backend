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
	client := candlepin_client.NewCandlepinClient()
	if args[1] == "init" {
		initCandlepin(client)
	} else if args[1] == "list-contents" {
		listContents(client)
	} else {
		log.Fatal().Msg(usage)
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
