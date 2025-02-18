package integration

import (
	"context"
	"embed"
	_ "embed"
	"fmt"
	"io/fs"
	"net/http"
	"sync"
	"time"

	"github.com/labstack/gommon/random"
	"github.com/rs/zerolog/log"
)

//go:embed fixtures
var fixturesContent embed.FS

type serveRepoOptions struct {
	port         string
	path         string
	repoSelector string
}

// Creates a random URL serving a yum repository
func ServeRandomYumRepo(options *serveRepoOptions) (url string, cancel context.CancelFunc, err error) {
	port := "30123"
	if options != nil && options.port != "" {
		port = options.port
	}

	path := fmt.Sprintf("/%v/", random.String(20))
	if options != nil && options.path != "" {
		path = options.path
	}

	// Make the root of the filesystem the contents of "giraffe", which will be served at path
	sub, err := fs.Sub(fixturesContent, "fixtures")
	if err != nil {
		return "", nil, err
	}

	repoContent, err := fs.Sub(sub, "giraffe")
	if err != nil {
		return "", nil, err
	}

	if options != nil && options.repoSelector == "frog" {
		repoContent, err = fs.Sub(sub, "frog")
		if err != nil {
			return "", nil, err
		}
	}
	mux := http.NewServeMux()

	mux.Handle(path, http.StripPrefix(path, http.FileServer(http.FS(repoContent))))

	server := &http.Server{
		Addr:              "pulp.content:" + port,
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

	return fmt.Sprintf("http://%v%v", server.Addr, path), cancel, err
}
