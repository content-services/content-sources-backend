package main

import (
	"os"
	"sort"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/external_repos"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/lib/pq"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

func main() {
	args := os.Args
	config.Load()
	err := db.Connect()
	if err != nil {
		log.Panic().Err(err).Msg("Failed to connect to database")
	}

	if len(args) < 2 {
		log.Fatal().Msg("Requires arguments: import.")
	}
	if args[1] == "download" {
		if len(args) < 3 {
			log.Fatal().Msg("Usage:  ./external-repos import /path/to/jsons/")
		}
		scanForExternalRepos(args[2])
	} else if args[1] == "import" {
		config.Load()
		err := db.Connect()
		if err != nil {
			log.Panic().Err(err).Msg("Failed to save repositories")
		}
		err = saveToDB(db.DB)
		if err != nil {
			log.Panic().Err(err).Msg("Failed to save repositories")
		}
		log.Debug().Msg("Successfully loaded external repositories.")
	} else if args[1] == "introspect" {
		if len(args) < 3 {
			log.Fatal().Msg("Usage:  ./external_repos introspect URL")
		}
		count, errors := external_repos.IntrospectUrl(args[2])
		for i := 0; i < len(errors); i++ {
			log.Panic().Err(errors[i]).Msg("Failed to introspect repository")
		}
		log.Debug().Msgf("Successfully Inserted %d packages", count)
	} else if args[1] == "introspect-all" {
		count, errors := external_repos.IntrospectAll()
		for i := 0; i < len(errors); i++ {
			log.Panic().Err(errors[i]).Msg("Failed to introspect repositories")
		}

		log.Debug().Msgf("Successfully Inserted %d packages", count)
	}
}

func saveToDB(db *gorm.DB) error {
	var (
		err      error
		extRepos []external_repos.ExternalRepository
		urls     []string
	)
	extRepos, err = external_repos.LoadFromFile()

	if err == nil {
		urls = external_repos.GetBaseURLs(extRepos)
		err = dao.GetRepositoryConfigDao(db).SavePublicRepos(urls)
	}
	return err
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
