package main

import (
	"flag"
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

var (
	forceIntrospect bool = false
)

func main() {
	args := os.Args
	config.Load()
	err := db.Connect()
	if err != nil {
		log.Panic().Err(err).Msg("Failed to connect to database")
	}

	if len(args) < 2 {
		log.Fatal().Msg("Requires arguments: download, import, introspect, introspect-all")
	}
	if args[1] == "download" {
		if len(args) < 3 {
			log.Fatal().Msg("Usage:  ./external-repos download /path/to/jsons/")
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
			log.Error().Msg("Usage:  ./external_repos introspect [--force] URL [URL2]...")
			os.Exit(1)
		}
		var urls []string
		for i := 2; i < len(args); i++ {
			if args[i] != "--force" {
				urls = append(urls, args[i])
			} else {
				forceIntrospect = true
			}
		}

		count, introErrors, errors := external_repos.IntrospectAll(&urls, forceIntrospect)
		for i := 0; i < len(introErrors); i++ {
			log.Warn().Msgf("Introspection Error: %v", introErrors[i].Error())
		}
		for i := 0; i < len(errors); i++ {
			log.Panic().Err(errors[i]).Msg("Failed to introspect repository due to fatal errors")
		}
		log.Debug().Msgf("Inserted %d packages", count)
	} else if args[1] == "introspect-all" {
		if len(args) > 2 {
			flagset := flag.NewFlagSet("introspect-all", flag.ExitOnError)

			flagset.BoolVar(&forceIntrospect, "force", false, "Force introspection even if not needed")
			if err = flagset.Parse(args[2:]); err != nil {
				log.Error().Err(err).Msg("Usage:  ./external_repos introspect-all [--force]")
				os.Exit(1)
			}
		}
		count, introErrors, errors := external_repos.IntrospectAll(nil, forceIntrospect)
		for i := 0; i < len(introErrors); i++ {
			log.Warn().Msgf("Introspection Error: %v", introErrors[i].Error())
		}
		for i := 0; i < len(errors); i++ {
			log.Error().Err(errors[i]).Msg("Fatal Introspection Error")
		}

		log.Debug().Msgf("Inserted %d packages", count)
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
