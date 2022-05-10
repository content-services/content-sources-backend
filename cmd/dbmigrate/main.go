package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/seeds"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/lib/pq"
)

func createMigrationFile(migrationName string) error {
	// datetime format in YYYYMMDDhhmmss - uses the reference time Mon Jan 2 15:04:05 MST 2006
	datetime := time.Now().Format("20060102150405")

	filenameUp := fmt.Sprintf("./db/migrations/%s_%s.up.sql", datetime, migrationName)
	filenameDown := fmt.Sprintf("./db/migrations/%s_%s.down.sql", datetime, migrationName)

	migrationTemplate := fmt.Sprintf("" +
		"BEGIN;\n" +
		"-- your migration here\n" +
		"COMMIT;\n")

	f, err := os.Create(filenameUp)
	if err != nil {
		return err
	}
	_, err = f.WriteString(migrationTemplate)
	if err != nil {
		return err
	}

	f, _ = os.Create(filenameDown)
	if err != nil {
		return err
	}
	_, err = f.WriteString(migrationTemplate)
	if err != nil {
		return err
	}

	f.Close()
	return err
}

func main() {

	upMigrationCmd := flag.NewFlagSet("up", flag.ExitOnError)
	upMigrationSteps := upMigrationCmd.Int("steps", 0, "migrate up")

	downMigrationCmd := flag.NewFlagSet("down", flag.ExitOnError)
	downMigrationSteps := downMigrationCmd.Int("steps", 0, "migrate down")

	dbURL := db.GetUrl()

	args := os.Args
	if len(args) < 2 {
		log.Fatalf("Requires arguments: up, down, or new.")
	}
	if args[1] == "new" {
		if err := createMigrationFile(args[2]); err != nil {
			log.Fatalf("%v", err)
		}
	} else if args[1] == "up" {
		if err := upMigrationCmd.Parse(args[2:]); err != nil {
			log.Fatalf("%v", err)
		}
		if err := db.MigrateDB(dbURL, "up", *upMigrationSteps); err != nil {
			log.Fatalf("%v", err)
		}
	} else if args[1] == "down" {
		if err := downMigrationCmd.Parse(args[2:]); err != nil {
			log.Fatalf("%v", err)
		}
		if err := db.MigrateDB(dbURL, "down", *downMigrationSteps); err != nil {
			log.Fatalf("%v", err)
		}
	} else if args[1] == "seed" {
		err := db.Connect()
		if err != nil {
			panic(err)
		}
		err = seeds.SeedRepositoryConfigurations(db.DB, 1000)
		if err != nil {
			panic(err)
		}
	}
}
