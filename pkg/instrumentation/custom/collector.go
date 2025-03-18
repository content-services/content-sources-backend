package custom

import (
	"context"
	"errors"
	"time"

	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/instrumentation"
	uuid2 "github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog/log"
	"golang.org/x/exp/slices"
	"gorm.io/gorm"
)

const tickerDelay = 30                     // in seconds // could be good to match this with the scrapper frequency
const snapshottingFailCheckDelay = 60 * 60 // in seconds

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
		c.metrics.RHCertExpiryDays.Set(float64(expire)) // TODO remove in favor of CertificateExpiryDays
		c.metrics.CertificateExpiryDays.WithLabelValues("cdn").Set(float64(expire))
	} else {
		log.Ctx(c.context).Error().Err(err).Msgf("Could not calculate cdn cert expiration")
	}

	var certUsers []config.CertUser
	if config.CandlepinConfigured() {
		certUsers = append(certUsers, &config.CandlepinCertUser{})
	}
	if config.FeatureServiceConfigured() {
		certUsers = append(certUsers, &config.FeatureServiceCertUser{})
	}
	log.Info().Msgf("featureServiceConfigured: %v", config.FeatureServiceConfigured())
	c.iterateCertUserExpiryTime(certUsers...)
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

	err := c.pulpClient.Livez(ctx)
	if err != nil {
		c.metrics.PulpConnectivity.Set(0)
	} else {
		c.metrics.PulpConnectivity.Set(1)
	}

	taskPendingTimeAverageByType := c.dao.TaskPendingTimeAverageByType(ctx)
	for _, t := range config.TaskTypes {
		value := 0.0
		indexFunc := func(a dao.TaskTypePendingTimeAverage) bool {
			return a.Type == t
		}
		if i := slices.IndexFunc(taskPendingTimeAverageByType, indexFunc); i >= 0 {
			value = taskPendingTimeAverageByType[i].PendingTime
		}
		c.metrics.TaskPendingTimeAverageByType.With(prometheus.Labels{"task_type": t}).Set(value)
	}

	templatesUseLatestCount := c.dao.TemplatesUseLatestCount(ctx)
	c.metrics.TemplatesUseLatestCount.Set(float64(templatesUseLatestCount))
	templatesUseDateCount := c.dao.TemplatesUseDateCount(ctx)
	c.metrics.TemplatesUseDateCount.Set(float64(templatesUseDateCount))
	templatesCount := templatesUseLatestCount + templatesUseDateCount
	c.metrics.TemplatesCount.Set(float64(templatesCount))
	templatesUpdatedCount := c.dao.TemplatesUpdatedInLast24HoursCount(ctx)
	c.metrics.TemplatesUpdatedInLast24HoursCount.Set(float64(templatesUpdatedCount))
	templatesAgeAverage := c.dao.TemplatesAgeAverage(ctx)
	c.metrics.TemplatesAgeAverage.Set(templatesAgeAverage)
}

func (c *Collector) snapshottingFailCheckIterate() {
	ctx := c.context
	c.metrics.RHReposSnapshotNotCompletedInLast36HoursCount.Set(float64(c.dao.RHReposSnapshotNotCompletedInLast36HoursCount(ctx)))
}

func (c *Collector) Run() {
	log.Info().Msg("Starting metrics collector go routine")
	ticker := time.NewTicker(tickerDelay * time.Second)
	snapshottingFailCheckTicker := time.NewTicker(snapshottingFailCheckDelay * time.Second)
	for {
		select {
		case <-ticker.C:
			c.iterate()
		case <-snapshottingFailCheckTicker.C:
			c.snapshottingFailCheckIterate()
		case <-c.context.Done():
			log.Info().Msgf("Stopping metrics collector go routine")
			ticker.Stop()
			return
		}
	}
}

func (c *Collector) iterateCertUserExpiryTime(certUsers ...config.CertUser) {
	for _, certUser := range certUsers {
		cert, err := config.GetCertificate(certUser)
		if err != nil {
			if errors.Is(err, ce.ErrCertKeyNotFound) {
				continue
			}
			log.Ctx(c.context).Error().Err(err).Msgf("Could not get %s certificate", certUser.Label())
		}

		expire, err := config.DaysTillExpiration(&cert)
		if err == nil {
			c.metrics.CertificateExpiryDays.WithLabelValues(certUser.Label()).Set(float64(expire))
		} else {
			log.Ctx(c.context).Error().Err(err).Msgf("Could not calculate %s cert expiration", certUser.Label())
		}
	}
}
