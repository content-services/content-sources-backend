package custom

import (
	"context"
	"time"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/instrumentation"
	"github.com/content-services/content-sources-backend/pkg/pulp_client"
	uuid2 "github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

const tickerDelay = 30 // in seconds // could be good to match this with the scrapper frequency

type Collector struct {
	context    context.Context
	metrics    *instrumentation.Metrics
	dao        dao.MetricsDao
	pulpClient pulp_client.PulpGlobalClient
}

func NewCollector(ctx context.Context, metrics *instrumentation.Metrics, db *gorm.DB, pulp pulp_client.PulpGlobalClient) *Collector {
	if ctx == nil {
		return nil
	}
	if metrics == nil {
		return nil
	}
	if db == nil {
		return nil
	}
	ctx = log.Logger.Level(config.MetricsLevel()).WithContext(ctx)
	ctx = context.WithValue(ctx, config.ContextRequestIDKey{}, uuid2.NewString())
	collector := &Collector{
		// Allow overriding metrics logging
		context:    ctx,
		metrics:    metrics,
		dao:        dao.GetMetricsDao(db),
		pulpClient: pulp,
	}
	collector.iterateExpiryTime() // iterate once to get accurate values
	return collector
}

func (c *Collector) iterateExpiryTime() {
	expire, err := config.CDNCertDaysTillExpiration()
	if err == nil {
		c.metrics.RHCertExpiryDays.Set(float64(expire))
	} else {
		log.Ctx(c.context).Error().Err(err).Msgf("Could not calculate cdn cert expiration")
	}
}

func (c *Collector) iterate() {
	ctx := c.context

	c.iterateExpiryTime()
	c.metrics.RepositoriesTotal.Set(float64(c.dao.RepositoriesCount(ctx)))
	c.metrics.RepositoryConfigsTotal.Set(float64(c.dao.RepositoryConfigsCount(ctx)))
	c.metrics.RepositoryConfigsTotal.Set(float64(c.dao.RepositoryConfigsCount(ctx)))
	c.metrics.OrgTotal.Set(float64(c.dao.OrganizationTotal(ctx)))

	public := c.dao.RepositoriesIntrospectionCount(ctx, 36, true)
	c.metrics.PublicRepositories36HourIntrospectionTotal.With(prometheus.Labels{"status": "introspected"}).Set(float64(public.Introspected))
	c.metrics.PublicRepositories36HourIntrospectionTotal.With(prometheus.Labels{"status": "missed"}).Set(float64(public.Missed))

	custom := c.dao.RepositoriesIntrospectionCount(ctx, 36, false)
	c.metrics.CustomRepositories36HourIntrospectionTotal.With(prometheus.Labels{"status": "introspected"}).Set(float64(custom.Introspected))
	c.metrics.CustomRepositories36HourIntrospectionTotal.With(prometheus.Labels{"status": "missed"}).Set(float64(custom.Missed))
	c.metrics.PublicRepositoriesWithFailedIntrospectionTotal.Set(float64(c.dao.PublicRepositoriesFailedIntrospectionCount(ctx)))

	latency := c.dao.PendingTasksAverageLatency(ctx)
	c.metrics.TaskStats.With(prometheus.Labels{"label": instrumentation.TaskStatsLabelAverageWait}).Set(latency)
	pendingCount := c.dao.PendingTasksCount(ctx)
	c.metrics.TaskStats.With(prometheus.Labels{"label": instrumentation.TaskStatsLabelPendingCount}).Set(float64(pendingCount))
	oldestQueuedSecs := c.dao.PendingTasksOldestTask(ctx)
	c.metrics.TaskStats.With(prometheus.Labels{"label": instrumentation.TaskStatsLabelOldestWait}).Set(oldestQueuedSecs)

	_, err := c.pulpClient.LookupDomain(ctx, pulp_client.DefaultDomain)
	if err != nil {
		c.metrics.PulpConnectivity.Set(0)
	} else {
		c.metrics.PulpConnectivity.Set(1)
	}
}

func (c *Collector) Run() {
	log.Info().Msg("Starting metrics collector go routine")
	ticker := time.NewTicker(tickerDelay * time.Second)
	for {
		select {
		case <-ticker.C:
			c.iterate()
		case <-c.context.Done():
			log.Info().Msgf("Stopping metrics collector go routine")
			ticker.Stop()
			return
		}
	}
}
