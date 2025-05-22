package commands

import (
	"sort"

	"github.com/content-services/content-sources-backend/pkg/external_repos"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v2"
)

func DownloadAction(c *cli.Context) error {
	path := c.String("path")
	scanForExternalRepos(path)
	return nil
}

func scanForExternalRepos(path string) {
	urls, err := external_repos.IBUrlsFromDir(path)
	if err != nil {
		log.Panic().Err(err).Msg("Failed to import repositories")
	}
	sort.Strings(urls)
	err = external_repos.SaveToFile(urls)
	if err != nil {
		log.Panic().Err(err).Msg("Failed to import repositories")
	}
	log.Info().Msg("Saved External Repositories")
}
