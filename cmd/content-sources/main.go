package main

import (
	"os"

	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/handler"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	golog "github.com/labstack/gommon/log"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/ziflex/lecho/v3"
)

func main() {
	e := echo.New()

	logLevel := golog.ERROR

	if true { //TODO: Dev debug based on env
		logLevel = golog.DEBUG
	}

	zl, el := lecho.MatchEchoLevel(logLevel)

	zerolog.SetGlobalLevel(zl)

	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	logger := lecho.New(
		log.Logger,
		lecho.WithLevel(el),
		// This prints the current debug level on each request
		// This level does not control whether a request is printed.
		lecho.WithTimestamp(),
		lecho.WithCaller(),
	)

	e.Use(middleware.RequestID())
	e.Use(lecho.Middleware(lecho.Config{
		Logger: logger,
	}))

	var err error

	err = db.Connect()
	if err != nil {
		log.Fatal().Err(err)
	}

	handler.RegisterRoutes(e)
	err = e.Start(":8000")
	if err != nil {
		log.Fatal().Err(err)
	}
}
