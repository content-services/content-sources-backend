package jfrog_bridge

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const metricsNamespace = "content_sources"
const metricsSubsystem = "bridge"
const partnerLabel = "partner"

// bridgeMetrics holds one metric family per counter, labelled by partner.
type bridgeMetrics struct {
	messagesReceived   *prometheus.CounterVec
	messagesProcessed  *prometheus.CounterVec
	messagesFailed     *prometheus.CounterVec
	embargoRejections  *prometheus.CounterVec
	checksumRejections *prometheus.CounterVec
	conflictRejections *prometheus.CounterVec
	pipelineDuration   *prometheus.HistogramVec
}

// partnerMetrics is the view of bridgeMetrics for a single partner.
type partnerMetrics struct {
	messagesReceived   prometheus.Counter
	messagesProcessed  prometheus.Counter
	messagesFailed     prometheus.Counter
	embargoRejections  prometheus.Counter
	checksumRejections prometheus.Counter
	conflictRejections prometheus.Counter
	pipelineDuration   prometheus.Observer
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
	labels := []string{partnerLabel}
	return &bridgeMetrics{
		messagesReceived: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "messages_received_total",
			Help:      "Total Kafka messages received by the content bridge",
		}, labels),
		messagesProcessed: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "messages_processed_total",
			Help:      "Total messages successfully processed end-to-end",
		}, labels),
		messagesFailed: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "messages_failed_total",
			Help:      "Total messages that failed pipeline processing",
		}, labels),
		embargoRejections: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "embargo_rejections_total",
			Help:      "Total remediations rejected by embargo gate",
		}, labels),
		checksumRejections: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "checksum_rejections_total",
			Help:      "Remediations rejected due to JAR checksum mismatch",
		}, labels),
		conflictRejections: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "conflict_rejections_total",
			Help:      "Remediations rejected because a partner object exists with different content",
		}, labels),
		pipelineDuration: factory.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "pipeline_duration_seconds",
			Help:      "Duration of the full pipeline per remediation",
			Buckets:   []float64{1, 5, 10, 30, 60, 120, 300},
		}, labels),
	}
}

func (m *bridgeMetrics) forPartner(name string) *partnerMetrics {
	return &partnerMetrics{
		messagesReceived:   m.messagesReceived.WithLabelValues(name),
		messagesProcessed:  m.messagesProcessed.WithLabelValues(name),
		messagesFailed:     m.messagesFailed.WithLabelValues(name),
		embargoRejections:  m.embargoRejections.WithLabelValues(name),
		checksumRejections: m.checksumRejections.WithLabelValues(name),
		conflictRejections: m.conflictRejections.WithLabelValues(name),
		pipelineDuration:   m.pipelineDuration.WithLabelValues(name),
	}
}
