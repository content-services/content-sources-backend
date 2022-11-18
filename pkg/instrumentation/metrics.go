package instrumentation

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type Metrics struct {
	repositoriesCreated  prometheus.Counter
	repositoriesDeleted  prometheus.Counter
	introspectionSuccess prometheus.Counter
	introspectionFailure prometheus.Counter
}

func NewMetrics() *Metrics {
	metrics := &Metrics{
		repositoriesCreated: promauto.NewCounter(prometheus.CounterOpts{
			Name: "repository_create_count",
			Help: "The number of repositories created",
		}),

		repositoriesDeleted: promauto.NewCounter(prometheus.CounterOpts{
			Name: "repository_delete_count",
			Help: "The number of repositories deleted",
		}),
		introspectionSuccess: promauto.NewCounter(prometheus.CounterOpts{
			Name: "introspection_success_count",
			Help: "The number of repositories introspected with success",
		}),
		introspectionFailure: promauto.NewCounter(prometheus.CounterOpts{
			Name: "introspection_failure_count",
			Help: "The number of repositories introspected with failure",
		}),
	}

	metrics.introspectionFailure.Add(0)
	metrics.introspectionSuccess.Add(0)

	return metrics
}

// FIXME Remove global variables
var (
	metrics = NewMetrics()
)
