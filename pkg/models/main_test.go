package models

import (
	"database/sql"
	"log"
	"os"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/db"
	pg "gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var dbConn *gorm.DB

func TestMain(m *testing.M) {
	//open database connection
	var err error
	dbURL := db.GetUrl()
	dbConn, err = gorm.Open(pg.Open(dbURL), &gorm.Config{})
	if err != nil {
		log.Fatalf("%v", err)
	}

	// run tests
	exitCode := m.Run()

	// close database connection
	var sqlDB *sql.DB
	sqlDB, err = dbConn.DB()

	if err != nil {
		log.Fatalf("%v", err)
	}

	if err = sqlDB.Close(); err != nil {
		log.Fatalf("%v", err)
	}
	os.Exit(exitCode)
}
