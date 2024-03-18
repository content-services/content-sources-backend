package custom

import (
	"context"
	"time"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/instrumentation"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

const tickerDelay = 5 // in seconds // could be good to match this with the scrapper frequency

type Collector struct {
	context context.Context
	metrics *instrumentation.Metrics
	dao     dao.MetricsDao
}

func NewCollector(context context.Context, metrics *instrumentation.Metrics, db *gorm.DB) *Collector {
	if context == nil {
		return nil
	}
	if metrics == nil {
		return nil
	}
	if db == nil {
		return nil
	}
	return &Collector{
		context: context,
		metrics: metrics,
		dao:     dao.GetMetricsDao(db),
	}
}

func (c *Collector) iterate() {
	ctx := context.Background()
	c.metrics.RepositoriesTotal.Set(float64(c.dao.RepositoriesCount(ctx)))
	c.metrics.RepositoryConfigsTotal.Set(float64(c.dao.RepositoryConfigsCount(ctx)))
	c.metrics.RepositoryConfigsTotal.Set(float64(c.dao.RepositoryConfigsCount(ctx)))
	c.metrics.OrgTotal.Set(float64(c.dao.OrganizationTotal(ctx)))
	c.metrics.RHCertExpiryDays.Set(float64(config.Get().Certs.DaysTillExpiration))

	public := c.dao.RepositoriesIntrospectionCount(ctx, 36, true)
	c.metrics.PublicRepositories36HourIntrospectionTotal.With(prometheus.Labels{"status": "introspected"}).Set(float64(public.Introspected))
	c.metrics.PublicRepositories36HourIntrospectionTotal.With(prometheus.Labels{"status": "missed"}).Set(float64(public.Missed))

	custom := c.dao.RepositoriesIntrospectionCount(ctx, 36, false)
	c.metrics.CustomRepositories36HourIntrospectionTotal.With(prometheus.Labels{"status": "introspected"}).Set(float64(custom.Introspected))
	c.metrics.CustomRepositories36HourIntrospectionTotal.With(prometheus.Labels{"status": "missed"}).Set(float64(custom.Missed))
	c.metrics.PublicRepositoriesWithFailedIntrospectionTotal.Set(float64(c.dao.PublicRepositoriesFailedIntrospectionCount(ctx)))
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
