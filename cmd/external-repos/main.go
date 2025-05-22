package main

import (
	"os"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/external_repos/commands"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v2"
)

func main() {
	config.Load()
	config.ConfigureLogging()

	err := db.Connect()
	if err != nil {
		log.Panic().Err(err).Msg("Failed to connect to database")
	}

	app := &cli.App{
		Name:  "external-repos",
		Usage: "Manage repositories",
		Commands: []*cli.Command{
			{
				Name:   "cleanup",
				Usage:  commands.CleanupUsage(),
				Action: commands.CleanupAction,
				Flags: []cli.Flag{
					&cli.StringSliceFlag{
						Name:  "type",
						Usage: "Run only the specified cleanup types",
					},
					&cli.StringSliceFlag{
						Name:  "exclude",
						Usage: "Do not run the specified cleanup types",
					},
					&cli.IntFlag{
						Name:        "pulp-orphan-batch-size",
						Usage:       "Batch size to use for pulp-orphan cleanup",
						DefaultText: "5",
					},
				},
			},
			{
				Name:  "download",
				Usage: "Download external repo urls",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "path",
						Usage: "Path to the JSON file to download from",
					},
				},
				Action: commands.DownloadAction,
			},
			{
				Name:   "import",
				Usage:  "Import external repo urls",
				Action: commands.ImportAction,
			},
			{
				Name:   "introspect",
				Usage:  "Introspect a repository",
				Action: commands.IntrospectAction,
				Flags: []cli.Flag{
					&cli.StringSliceFlag{
						Name:     "url",
						Usage:    "URLs of repositories to introspect",
						Required: true,
					},
					&cli.StringFlag{
						Name:  "force",
						Usage: "Force introspection of the repository",
					},
				},
			},
			{
				Name:   "process-repos",
				Usage:  "Run the process-repos tasks",
				Action: commands.ProcessReposAction,
				Flags: []cli.Flag{
					&cli.IntFlag{
						Name:  "interval",
						Usage: "Number of times a day to run process repos",
					},
				},
			},
			{
				Name:   "snapshot",
				Usage:  "Snapshot a repository",
				Action: commands.SnapshotAction,
				Flags: []cli.Flag{
					&cli.StringSliceFlag{
						Name:     "url",
						Usage:    "URLs of repositories to snapshot",
						Required: true,
					},
					&cli.StringFlag{
						Name:  "force",
						Usage: "Force snapshot of the repository",
					},
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Error().Msgf("error: %v", err)
	}
}
