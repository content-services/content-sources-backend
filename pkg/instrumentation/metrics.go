package instrumentation

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type Metrics struct {
	repositoriesCreated  prometheus.Counter
	repositoriesDeleted  prometheus.Counter
	introspectionSuccess prometheus.Counter
	introspectionFailure prometheus.Counter
}

func NewMetrics(reg *prometheus.Registry) *Metrics {
	// FIXME promauto is using prometheus.DefaultRegisterer
	// TODO Update metric names according to: https://prometheus.io/docs/instrumenting/writing_exporters/#naming
	metrics := &Metrics{
		repositoriesCreated: promauto.With(reg).NewCounter(prometheus.CounterOpts{
			Name: "repository_create_count",
			Help: "The number of repositories created",
		}),

		repositoriesDeleted: promauto.With(reg).NewCounter(prometheus.CounterOpts{
			Name: "repository_delete_count",
			Help: "The number of repositories deleted",
		}),
		introspectionSuccess: promauto.With(reg).NewCounter(prometheus.CounterOpts{
			Name: "introspection_success_count",
			Help: "The number of repositories introspected with success",
		}),
		introspectionFailure: promauto.With(reg).NewCounter(prometheus.CounterOpts{
			Name: "introspection_failure_count",
			Help: "The number of repositories introspected with failure",
		}),
	}

	// reg.MustRegister(metrics.introspectionSuccess)
	// reg.MustRegister(metrics.introspectionFailure)
	// reg.MustRegister(metrics.repositoriesCreated)
	// reg.MustRegister(metrics.repositoriesDeleted)

	reg.MustRegister(collectors.NewBuildInfoCollector())

	return metrics
}
