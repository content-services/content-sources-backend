package instrumentation

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// TODO Update metric names according to: https://prometheus.io/docs/instrumenting/writing_exporters/#naming
const (
	HttpTotalRequests          = "http_total_requests"
	HttpTotalFailedRequests    = "http_total_failed_requests"
	HttpRequestDurationSeconds = "http_request_duration_seconds"
)

type Metrics struct {
	// serviceTotalRepositories                    prometheus.Gauge
	// serviceTotalRepositoryConfigs               prometheus.Gauge
	// servicePublicNotIntrospectedRepositories    prometheus.Gauge
	// servicePublicFailedIntrospection            prometheus.Gauge
	// serviceNonPublicNotIntrospectedRepositories prometheus.Gauge
	// serviceTop50Repositories                    prometheus.Gauge

	HttpTotalRequests       prometheus.Counter
	HttpTotalFailedRequests prometheus.Counter
	HttpRequestLatency      prometheus.HistogramVec
}

// See: https://consoledot.pages.redhat.com/docs/dev/platform-documentation/understanding-slo.html
func NewMetrics(reg *prometheus.Registry) *Metrics {
	metrics := &Metrics{
		HttpTotalRequests: promauto.With(reg).NewCounter(prometheus.CounterOpts{
			Name: HttpTotalRequests,
			Help: "The number of http requests made",
		}),
		HttpTotalFailedRequests: promauto.With(reg).NewCounter(prometheus.CounterOpts{
			Name: HttpTotalFailedRequests,
			Help: "The number of http requests made that resulted in a 5XX error",
		}),
		HttpRequestLatency: *promauto.With(reg).NewHistogramVec(prometheus.HistogramOpts{
			Name:    HttpRequestDurationSeconds,
			Help:    "Latency of request in seconds.",
			Buckets: prometheus.LinearBuckets(0.01, 0.05, 10),
		}, []string{"status", "method", "path"}),
		// serviceTotalRepositories: promauto.With(reg).NewGauge(prometheus.GaugeOpts{
		// 	Name: "service_repositories_total",
		// 	Help: "Total number of repositories",
		// }),
		// serviceTotalRepositoryConfigs: promauto.With(reg).NewGauge(prometheus.GaugeOpts{
		// 	Name: "service_repository_configs_total",
		// 	Help: "Total number of repository configs",
		// }),
		// servicePublicNotIntrospectedRepositories: promauto.With(reg).NewGauge(prometheus.GaugeOpts{
		// 	Name: "service_public_not_introspected_repositories_total",
		// 	Help: "Number of public repositories that have not attempted introspection in the last 24 hours",
		// }),
		// servicePublicFailedIntrospection: promauto.With(reg).NewGauge(prometheus.GaugeOpts{
		// 	Name: "service_public_failed_introspection_total",
		// 	Help: "Number of public repositories with failed introspections",
		// }),
		// serviceNonPublicNotIntrospectedRepositories: promauto.With(reg).NewGauge(prometheus.GaugeOpts{
		// 	Name: "service_non_public_not_introspected_repositories_total",
		// 	Help: "Number of non public repositories that have not attempted introspection in the last 24 hours",
		// }),
		// serviceTop50Repositories: promauto.With(reg).NewGauge(prometheus.GaugeOpts{
		// 	Name: "service_top50_repositories",
		// 	Help: "Number of non public repositories that have not attempted introspection in the last 24 hours",
		// }),
	}

	reg.MustRegister(collectors.NewBuildInfoCollector())

	return metrics
}
