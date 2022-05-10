package main

import (
	"log"

	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/handler"
	"github.com/labstack/echo/v4"
)

func main() {
	var err error

	err = db.Connect()
	if err != nil {
		log.Fatalf("%v", err)
	}

	e := echo.New()
	handler.RegisterRoutes(e)
	err = e.Start(":8000")
	if err != nil {
		log.Fatal(err)
	}
}
