package instrumentation

import (
	"context"
	"time"

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

func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.metrics.RepositoriesTotal.Desc()
	ch <- c.metrics.RepositoryConfigsTotal.Desc()
	ch <- c.metrics.PublicRepositoriesNotIntrospectedLast24HoursTotal.Desc()
	ch <- c.metrics.PublicRepositoriesWithFailedIntrospectionTotal.Desc()
	ch <- c.metrics.NonPublicRepositoriesNotIntrospectedLast24HoursTotal.Desc()
}

func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	c.metrics.RepositoriesTotal.Collect(ch)
	c.metrics.RepositoryConfigsTotal.Collect(ch)
	c.metrics.PublicRepositoriesNotIntrospectedLast24HoursTotal.Collect(ch)
	c.metrics.PublicRepositoriesWithFailedIntrospectionTotal.Collect(ch)
	c.metrics.NonPublicRepositoriesNotIntrospectedLast24HoursTotal.Collect(ch)
}

func (c *Collector) iterateTop50Repositories() {
	// for _, row := range c.dao.Top50Repositories() {
	// 	url := row["url"].(string)
	// 	repo_count := row["repo_count"].(int64)
	// 	c.metrics.Top50Repositories.WithLabelValues(
	// 		url,
	// 	).Set(float64(repo_count))
	// }
}

func (c *Collector) iterate() {
	c.metrics.RepositoriesTotal.Set(float64(c.dao.RepositoriesCount()))
	c.metrics.RepositoryConfigsTotal.Set(float64(c.dao.RepositoryConfigsCount()))
	c.metrics.PublicRepositoriesNotIntrospectedLast24HoursTotal.Set(float64(c.dao.PublicRepositoriesNotIntrospectedLas24HoursCount()))
	c.metrics.PublicRepositoriesWithFailedIntrospectionTotal.Set(float64(c.dao.PublicRepositoriesFailedIntrospectionCount()))
	c.metrics.NonPublicRepositoriesNotIntrospectedLast24HoursTotal.Set(float64(c.dao.NonPublicRepositoriesNonIntrospectedLast24HoursCount()))
	c.iterateTop50Repositories()
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
