package integration

import (
	"context"
	"embed"
	_ "embed"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/labstack/gommon/random"
	"github.com/rs/zerolog/log"
)

//go:embed data
var repoContent embed.FS

// Creates a random URL serving a yum repository
func ServeRandomYumRepo() (url string, cancel context.CancelFunc, err error) {
	path := fmt.Sprintf("/%v/", random.String(20))
	mux := http.NewServeMux()

	mux.Handle(path, http.StripPrefix(path, http.FileServer(http.FS(repoContent))))

	server := &http.Server{
		Addr:              "pulp.content:30123",
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		err = server.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			fmt.Printf("Error starting server: %v", err)
		}
		wg.Done()
	}()
	cancel = func() {
		err := server.Shutdown(context.Background())
		if err != nil {
			log.Error().Err(err).Msg("Error shutting down yum server gracefully")
		}
		wg.Wait()
	}

	return fmt.Sprintf("http://%v%v%v/", server.Addr, path, "data"), cancel, err
}
