package jfrog_bridge

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const metricsNamespace = "content_sources"
const metricsSubsystem = "jfrog_bridge"

type bridgeMetrics struct {
	messagesReceived  prometheus.Counter
	messagesProcessed prometheus.Counter
	messagesFailed    prometheus.Counter
	pipelineDuration  prometheus.Histogram
}

var (
	sharedMetrics     *bridgeMetrics
	sharedMetricsOnce sync.Once
)

// getSharedMetrics returns a singleton metrics instance registered with
// the default Prometheus registerer (scraped via /metrics).
func getSharedMetrics() *bridgeMetrics {
	sharedMetricsOnce.Do(func() {
		sharedMetrics = newBridgeMetrics(nil)
	})
	return sharedMetrics
}

func newBridgeMetrics(reg prometheus.Registerer) *bridgeMetrics {
	if reg == nil {
		reg = prometheus.DefaultRegisterer
	}
	factory := promauto.With(reg)
	return &bridgeMetrics{
		messagesReceived: factory.NewCounter(prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "messages_received_total",
			Help:      "Total Kafka messages received by the JFrog bridge",
		}),
		messagesProcessed: factory.NewCounter(prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "messages_processed_total",
			Help:      "Total messages successfully processed end-to-end",
		}),
		messagesFailed: factory.NewCounter(prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "messages_failed_total",
			Help:      "Total messages that failed pipeline processing",
		}),
		pipelineDuration: factory.NewHistogram(prometheus.HistogramOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "pipeline_duration_seconds",
			Help:      "Duration of the full pipeline per remediation",
			Buckets:   []float64{1, 5, 10, 30, 60, 120, 300},
		}),
	}
}
