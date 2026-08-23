package jfrog_bridge

import (
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

func newBridgeMetrics(reg *prometheus.Registry) *bridgeMetrics {
	if reg == nil {
		reg = prometheus.NewRegistry()
	}
	return &bridgeMetrics{
		messagesReceived: promauto.With(reg).NewCounter(prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "messages_received_total",
			Help:      "Total Kafka messages received by the JFrog bridge",
		}),
		messagesProcessed: promauto.With(reg).NewCounter(prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "messages_processed_total",
			Help:      "Total messages successfully processed end-to-end",
		}),
		messagesFailed: promauto.With(reg).NewCounter(prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "messages_failed_total",
			Help:      "Total messages that failed pipeline processing",
		}),
		pipelineDuration: promauto.With(reg).NewHistogram(prometheus.HistogramOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "pipeline_duration_seconds",
			Help:      "Duration of the full pipeline per remediation",
			Buckets:   []float64{1, 5, 10, 30, 60, 120, 300},
		}),
	}
}
