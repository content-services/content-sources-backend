package instrumentation

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	CoprBuildEndEventsTotal = "copr_build_end_events_total" // build end events
)

type CoprBuildEndIngestOutcome string

const (
	CoprBuildEndIngestEnqueued CoprBuildEndIngestOutcome = "enqueued" // snapshot task enqueued
	CoprBuildEndIngestInvalid  CoprBuildEndIngestOutcome = "invalid"  // skipped, the message content is invalid
	CoprBuildEndIngestFailed   CoprBuildEndIngestOutcome = "failed"   // skipped, handling a valid message failed (e.g., enqueue failed)
)

type CoprIngestMetrics struct {
	CoprBuildEndEventsTotal prometheus.CounterVec
}

var coprBuildEndIngestOutcomes = []CoprBuildEndIngestOutcome{
	CoprBuildEndIngestEnqueued,
	CoprBuildEndIngestInvalid,
	CoprBuildEndIngestFailed,
}

func newCoprIngestMetrics(reg *prometheus.Registry) CoprIngestMetrics {
	m := CoprIngestMetrics{
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

// RecordCoprBuildEndIngest counts a handled Copr build end event by its ingest outcome.
func (m *Metrics) RecordCoprBuildEndIngest(outcome CoprBuildEndIngestOutcome) {
	if m != nil {
		m.CoprBuildEndEventsTotal.With(prometheus.Labels{"outcome": string(outcome)}).Inc()
	}
}
