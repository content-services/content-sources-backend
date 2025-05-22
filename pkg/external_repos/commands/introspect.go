package commands

import (
	"context"

	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/external_repos"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v2"
)

func IntrospectAction(c *cli.Context) error {
	ctx := c.Context
	force := c.Bool("force")
	urls := c.StringSlice("url")

	introspectUrls(ctx, urls, force)
	return nil
}

func introspectUrls(ctx context.Context, urls []string, force bool) {
	repos, err := dao.GetDaoRegistry(db.DB).Repository.ListForIntrospection(ctx, &urls, force)
	if err != nil {
		log.Fatal().Err(err).Msg("Could not lookup repos to introspect")
	}
	for _, repo := range repos {
		count, introError, error := external_repos.IntrospectUrl(ctx, repo.URL)
		if introError != nil {
			log.Warn().Msgf("Introspection Error: %v", introError)
		}
		if error != nil {
			log.Panic().Err(error).Msg("Failed to introspect repository due to fatal errors")
		}
		log.Debug().Msgf("Inserted %d packages for %v", count, repo.URL)
	}
}
