package instrumentation

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// TODO Update metric names according to: https://prometheus.io/docs/instrumenting/writing_exporters/#naming
const (
	NameSpace                                            = "content_sources"
	HttpStatusHistogram                                  = "http_status_histogram"
	RepositoriesTotal                                    = "repositories_total"
	RepositoryConfigsTotal                               = "repository_configs_total"
	PublicRepositoriesNotIntrospectedLast24HoursTotal    = "public_repositories_not_introspected_last_24_hours_total"
	PublicRepositoriesWithFailedIntrospectionTotal       = "public_repositories_with_failed_introspection_total"
	NonPublicRepositoriesNotIntrospectedLast24HoursTotal = "non_public_repositories_not_introspected_last_24_hours_total"
	Top50Repositories                                    = "top_50_repositories"
)

type Metrics struct {
	HttpStatusHistogram prometheus.HistogramVec

	// Custom metrics
	RepositoriesTotal                                    prometheus.Gauge
	RepositoryConfigsTotal                               prometheus.Gauge
	PublicRepositoriesNotIntrospectedLast24HoursTotal    prometheus.Gauge
	PublicRepositoriesWithFailedIntrospectionTotal       prometheus.Gauge
	NonPublicRepositoriesNotIntrospectedLast24HoursTotal prometheus.Gauge
	Top50Repositories                                    prometheus.GaugeVec

	reg *prometheus.Registry
}

// See: https://consoledot.pages.redhat.com/docs/dev/platform-documentation/understanding-slo.html
// See: https://prometheus.io/docs/tutorials/understanding_metric_types/#types-of-metrics
func NewMetrics(reg *prometheus.Registry) *Metrics {
	if reg == nil {
		panic("reg cannot be nil")
	}
	metrics := &Metrics{
		reg: reg,
		HttpStatusHistogram: *promauto.With(reg).NewHistogramVec(prometheus.HistogramOpts{
			Namespace: NameSpace,
			Name:      HttpStatusHistogram,
			Help:      "Duration of HTTP requests",
			Buckets:   prometheus.DefBuckets,
		}, []string{"status", "method", "path"}),

		RepositoriesTotal: promauto.With(reg).NewGauge(prometheus.GaugeOpts{
			Namespace: NameSpace,
			Name:      RepositoriesTotal,
			Help:      "Number of repositories",
		}),
		RepositoryConfigsTotal: promauto.With(reg).NewGauge(prometheus.GaugeOpts{
			Namespace: NameSpace,
			Name:      RepositoryConfigsTotal,
			Help:      "Number of repository configurations",
		}),
		PublicRepositoriesNotIntrospectedLast24HoursTotal: promauto.With(reg).NewGauge(prometheus.GaugeOpts{
			Namespace: NameSpace,
			Name:      PublicRepositoriesNotIntrospectedLast24HoursTotal,
			Help:      "Number of public repositories not introspected into the last 24 hours",
		}),
		PublicRepositoriesWithFailedIntrospectionTotal: promauto.With(reg).NewGauge(prometheus.GaugeOpts{
			Namespace: NameSpace,
			Name:      PublicRepositoriesWithFailedIntrospectionTotal,
			Help:      "Number of repositories with failed introspection",
		}),
		NonPublicRepositoriesNotIntrospectedLast24HoursTotal: promauto.With(reg).NewGauge(prometheus.GaugeOpts{
			Namespace: NameSpace,
			Name:      NonPublicRepositoriesNotIntrospectedLast24HoursTotal,
			Help:      "Number of non public repositories not introspected in the last 24 hours",
		}),
		Top50Repositories: *promauto.With(reg).NewGaugeVec(prometheus.GaugeOpts{
			Namespace: NameSpace,
			Name:      Top50Repositories,
			Help:      "The top 50 repositories",
		}, []string{"url"}),
	}

	reg.MustRegister(collectors.NewBuildInfoCollector())

	return metrics
}

func (m Metrics) Registry() *prometheus.Registry {
	return m.reg
}
