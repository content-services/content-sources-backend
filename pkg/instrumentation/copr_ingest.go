package instrumentation

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	CoprEventDelay          = "copr_event_delay_seconds"    // shared by all event types on the topic
	CoprBuildEndEventsTotal = "copr_build_end_events_total" // build end events
)

type CoprBuildEndIngestOutcome string

const (
	CoprBuildEndIngestEnqueued CoprBuildEndIngestOutcome = "enqueued" // snapshot task enqueued
	CoprBuildEndIngestInvalid  CoprBuildEndIngestOutcome = "invalid"  // skipped, the message content is invalid
	CoprBuildEndIngestFailed   CoprBuildEndIngestOutcome = "failed"   // skipped, handling a valid message failed (e.g., enqueue failed)
)

type CoprIngestMetrics struct {
	CoprEventDelay          prometheus.Histogram
	CoprBuildEndEventsTotal prometheus.CounterVec
}

var coprBuildEndIngestOutcomes = []CoprBuildEndIngestOutcome{
	CoprBuildEndIngestEnqueued,
	CoprBuildEndIngestInvalid,
	CoprBuildEndIngestFailed,
}

func newCoprIngestMetrics(reg *prometheus.Registry) CoprIngestMetrics {
	m := CoprIngestMetrics{
		CoprEventDelay: promauto.With(reg).NewHistogram(prometheus.HistogramOpts{
			Namespace: NameSpace,
			Name:      CoprEventDelay,
			Help:      "Time from when Copr sent an event to when it was handled",
			//                               1m  5m   15m  1h    6h
			Buckets: []float64{1, 5, 15, 30, 60, 300, 900, 3600, 21600},
		}),
		CoprBuildEndEventsTotal: *promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
			Namespace: NameSpace,
			Name:      CoprBuildEndEventsTotal,
			Help:      "Total Copr build end events by ingest outcome (enqueued, invalid, failed). Failed builds and unsupported chroots aren't counted",
		}, []string{"outcome"}),
	}
	// Initialize every outcome label to 0 so its series is exported from startup, not only after the first event
	for _, outcome := range coprBuildEndIngestOutcomes {
		m.CoprBuildEndEventsTotal.WithLabelValues(string(outcome))
	}
	return m
}

// RecordCoprEventDelay observes the time since a Copr event was sent. Events without a time are not observed.
func (m *Metrics) RecordCoprEventDelay(sent time.Time) {
	if m == nil || sent.IsZero() {
		return
	}
	m.CoprEventDelay.Observe(max(time.Since(sent).Seconds(), 0)) // clock skew can make the delay negative
}

// RecordCoprBuildEndIngest counts a handled Copr build end event by its ingest outcome.
func (m *Metrics) RecordCoprBuildEndIngest(outcome CoprBuildEndIngestOutcome) {
	if m != nil {
		m.CoprBuildEndEventsTotal.With(prometheus.Labels{"outcome": string(outcome)}).Inc()
	}
}
