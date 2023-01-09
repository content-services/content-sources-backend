package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/labstack/echo/v4"
	echo_middleware "github.com/labstack/echo/v4/middleware"
	"github.com/rs/zerolog/log"
)

func main() {
	var (
		wg sync.WaitGroup
	)
	ctx, cancel := context.WithCancel(context.Background())
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	e := echo.New()
	e.Use(
		echo_middleware.Logger(),
		echo_middleware.Recover(),
	)
	e.Add(echo.GET, RbacV1Access, MockRbac)
	wg.Add(1)
	go func() {
		defer wg.Done()
		log.Info().Msgf("mock service starting")
		err := e.Start(":8800")
		if err != nil && err != http.ErrServerClosed {
			log.Fatal().Msgf("error starting service: %s", err.Error())
		}
		log.Info().Msgf("mock service stopped")
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		<-ctx.Done()
		log.Logger.Info().Msgf("stopping mock service")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		if err := e.Shutdown(ctx); err != nil {
			log.Fatal().Msgf("error shutting down mock service: %s", err.Error())
		}
		cancel()
	}()

	<-quit
	cancel()
	wg.Wait()
}
