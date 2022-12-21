package instrumentation

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
)

func TestNewMetrics(t *testing.T) {
	var (
		reg     *prometheus.Registry
		metrics *Metrics
	)
	assert.Panics(t, func() {
		metrics = NewMetrics(nil)
	})

	reg = prometheus.NewRegistry()

	metrics = NewMetrics(reg)
	assert.NotNil(t, metrics)
}

func TestRegistry(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics := NewMetrics(reg)
	assert.NotNil(t, metrics)
	assert.Equal(t, reg, metrics.Registry())
}
