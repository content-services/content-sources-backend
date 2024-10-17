package integration

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/handler"
	m "github.com/content-services/content-sources-backend/pkg/instrumentation"
	"github.com/content-services/content-sources-backend/pkg/router"
	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type HealthCheckSuite struct {
	Suite
	ctx           context.Context
	metricsServer *http.Server
	pingServer    *http.Server
	cancel        context.CancelFunc
}

func (s *HealthCheckSuite) SetupTest() {
	s.Suite.SetupTest()
	s.ctx, s.cancel = context.WithCancel(context.Background())

	// set up ping
	pingRouter := echo.New()
	handler.RegisterPing(pingRouter)

	config.Get().Metrics.Path = "/metrics"
	config.Get().Metrics.Port = 9005

	testReg := prometheus.NewRegistry()
	metrics := m.NewMetrics(testReg)
	metricsRouter := router.ConfigureEcho(false)
	metricsRouter.Add(http.MethodGet, config.Get().Metrics.Path, echo.WrapHandler(promhttp.HandlerFor(
		metrics.Registry(),
		promhttp.HandlerOpts{
			// Opt into OpenMetrics to support exemplars.
			EnableOpenMetrics: true,
			// Pass custom registry
			Registry: metrics.Registry(),
		},
	)))

	s.pingServer = &http.Server{
		Addr:              "127.0.0.1:8005",
		Handler:           pingRouter,
		IdleTimeout:       1 * time.Minute,
		ReadTimeout:       5 * time.Second,
		ReadHeaderTimeout: 2 * time.Second,
		WriteTimeout:      10 * time.Second,
	}

	s.metricsServer = &http.Server{
		Addr:              "127.0.0.1:9005",
		Handler:           metricsRouter,
		IdleTimeout:       1 * time.Minute,
		ReadTimeout:       5 * time.Second,
		ReadHeaderTimeout: 2 * time.Second,
		WriteTimeout:      10 * time.Second,
	}
}

func (s *HealthCheckSuite) TearDownTest() {
	s.cancel()
	err := s.metricsServer.Shutdown(context.Background())
	if err != nil {
		log.Error().Err(err).Msg("Could not shutdown server")
	}
	err = s.pingServer.Shutdown(context.Background())
	if err != nil {
		log.Error().Err(err).Msg("Could not shutdown server")
	}
	s.Suite.TearDownTest()
}

func (s *HealthCheckSuite) serveRouter(req *http.Request, server *http.Server) (int, []byte, error) {
	rr := httptest.NewRecorder()
	server.Handler.ServeHTTP(httptest.NewRecorder(), req)

	response := rr.Result()
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	require.NoError(s.T(), err)

	return response.StatusCode, body, err
}

func TestHealthcheckSuite(t *testing.T) {
	suite.Run(t, new(HealthCheckSuite))
}

func (s *HealthCheckSuite) TestMetricsStatus() {
	t := s.T()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set("Content-Type", "application/json")

	code, _, err := s.serveRouter(req, s.metricsServer)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)
}

func (s *HealthCheckSuite) TestPingStatus() {
	t := s.T()
	req := httptest.NewRequest(http.MethodGet, "/ping/", nil)
	req.Header.Set("Content-Type", "application/json")

	code, _, err := s.serveRouter(req, s.pingServer)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, code)
}
