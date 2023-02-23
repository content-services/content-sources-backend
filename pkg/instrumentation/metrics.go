package instrumentation

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// TODO Update metric names according to: https://prometheus.io/docs/instrumenting/writing_exporters/#naming
const (
	NameSpace                                      = "content_sources"
	HttpStatusHistogram                            = "http_status_histogram"
	RepositoriesTotal                              = "repositories_total"
	RepositoryConfigsTotal                         = "repository_configs_total"
	PublicRepositories36HourIntrospectionTotal     = "public_repositories_36_hour_introspection_total"
	PublicRepositoriesWithFailedIntrospectionTotal = "public_repositories_with_failed_introspection_total"
	CustomRepositories36HourIntrospectionTotal     = "custom_repositories_36_hour_introspection_total"
	KafkaMessageLatency                            = "kafka_message_latency"
	KafkaMessageStatus                             = "kafka_message_status"
)

type Metrics struct {
	HttpStatusHistogram prometheus.HistogramVec

	// Custom metrics
	RepositoriesTotal                              prometheus.Gauge
	RepositoryConfigsTotal                         prometheus.Gauge
	PublicRepositories36HourIntrospectionTotal     prometheus.GaugeVec
	PublicRepositoriesWithFailedIntrospectionTotal prometheus.Gauge
	CustomRepositories36HourIntrospectionTotal     prometheus.GaugeVec
	KafkaMessageStatus                             prometheus.CounterVec
	KafkaMessageLatency                            prometheus.Histogram

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

		KafkaMessageLatency: promauto.With(reg).NewHistogram(prometheus.HistogramOpts{
			Namespace: NameSpace,
			Name:      KafkaMessageLatency,
			Help:      "Time to pickup kafka messages",
			Buckets:   prometheus.DefBuckets,
		}),
		KafkaMessageStatus: *promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
			Namespace:   NameSpace,
			Name:        KafkaMessageStatus,
			Help:        "Result of kafka messages",
			ConstLabels: nil,
		}, []string{"state"}),
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
		PublicRepositories36HourIntrospectionTotal: *promauto.With(reg).NewGaugeVec(prometheus.GaugeOpts{
			Namespace: NameSpace,
			Name:      PublicRepositories36HourIntrospectionTotal,
			Help:      "Number of public repositories not introspected into the last 24 hours",
		}, []string{"status"}),
		PublicRepositoriesWithFailedIntrospectionTotal: promauto.With(reg).NewGauge(prometheus.GaugeOpts{
			Namespace: NameSpace,
			Name:      PublicRepositoriesWithFailedIntrospectionTotal,
			Help:      "Number of repositories with failed introspection",
		}),
		CustomRepositories36HourIntrospectionTotal: *promauto.With(reg).NewGaugeVec(prometheus.GaugeOpts{
			Namespace: NameSpace,
			Name:      CustomRepositories36HourIntrospectionTotal,
			Help:      "Number of non public repositories not introspected in the last 24 hours",
		}, []string{"status"}),
	}

	reg.MustRegister(collectors.NewBuildInfoCollector())

	return metrics
}

func (m *Metrics) RecordKafkaMessageStatus(success bool) {
	status := "failed"
	if success {
		status = "success"
	}
	if m != nil {
		m.KafkaMessageStatus.With(prometheus.Labels{"state": status}).Inc()
	}
}
func (m *Metrics) RecordKafkaLatency(msgTime time.Time) {
	diff := time.Since(msgTime)
	m.KafkaMessageLatency.Observe(diff.Seconds())
}

func (m Metrics) Registry() *prometheus.Registry {
	return m.reg
}
