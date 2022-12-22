package instrumentation

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// TODO Update metric names according to: https://prometheus.io/docs/instrumenting/writing_exporters/#naming
const (
	HttpStatusHistogram = "http_status_histogram"
)

type Metrics struct {
	HttpStatusHistogram prometheus.HistogramVec

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
			Name:    HttpStatusHistogram,
			Help:    "Duration of HTTP requests",
			Buckets: prometheus.LinearBuckets(0.01, 0.05, 10),
		}, []string{"status", "method", "path"}),
	}

	reg.MustRegister(collectors.NewBuildInfoCollector())

	return metrics
}

func (m Metrics) Registry() *prometheus.Registry {
	return m.reg
}
