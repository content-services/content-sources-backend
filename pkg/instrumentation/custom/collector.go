package custom

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/instrumentation"
	uuid2 "github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

const tickerDelay = 30                     // in seconds // could be good to match this with the scrapper frequency
const snapshottingFailCheckDelay = 60 * 60 // in seconds

type Collector struct {
	context    context.Context
	metrics    *instrumentation.Metrics
	dao        *dao.DaoRegistry
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
		dao:        dao.GetDaoRegistry(db),
		pulpClient: pulp,
	}
	collector.iterateExpiryTime() // iterate once to get accurate values
	return collector
}

func (c *Collector) iterateExpiryTime() {
	expire, err := config.CDNCertDaysTillExpiration()
	if err == nil {
		c.metrics.CertificateExpiryDays.WithLabelValues("cdn").Set(float64(expire))
	} else {
		log.Ctx(c.context).Error().Err(err).Msgf("Could not calculate cdn cert expiration")
	}
	certUsers := config.CertUsers()
	c.iterateCertUserExpiryTime(certUsers...)
}

func (c *Collector) iterate() {
	ctx := c.context
	metricsDao := c.dao.Metrics

	c.iterateExpiryTime()
	c.metrics.RepositoriesTotal.Set(float64(metricsDao.RepositoriesCount(ctx)))
	c.metrics.RepositoryConfigsTotal.Set(float64(metricsDao.RepositoryConfigsCount(ctx)))
	c.metrics.RepositoryConfigsTotal.Set(float64(metricsDao.RepositoryConfigsCount(ctx)))
	c.metrics.OrgTotal.Set(float64(metricsDao.OrganizationTotal(ctx)))

	public := metricsDao.RepositoriesIntrospectionCount(ctx, 36, true)
	c.metrics.PublicRepositories36HourIntrospectionTotal.With(prometheus.Labels{"status": "introspected"}).Set(float64(public.Introspected))
	c.metrics.PublicRepositories36HourIntrospectionTotal.With(prometheus.Labels{"status": "missed"}).Set(float64(public.Missed))

	custom := metricsDao.RepositoriesIntrospectionCount(ctx, 36, false)
	c.metrics.CustomRepositories36HourIntrospectionTotal.With(prometheus.Labels{"status": "introspected"}).Set(float64(custom.Introspected))
	c.metrics.CustomRepositories36HourIntrospectionTotal.With(prometheus.Labels{"status": "missed"}).Set(float64(custom.Missed))
	c.metrics.PublicRepositoriesWithFailedIntrospectionTotal.Set(float64(metricsDao.PublicRepositoriesFailedIntrospectionCount(ctx)))

	latency := metricsDao.PendingTasksAverageLatency(ctx)
	c.metrics.TaskStats.With(prometheus.Labels{"label": instrumentation.TaskStatsLabelAverageWait}).Set(latency)
	pendingCount := metricsDao.PendingTasksCount(ctx)
	c.metrics.TaskStats.With(prometheus.Labels{"label": instrumentation.TaskStatsLabelPendingCount}).Set(float64(pendingCount))
	oldestQueuedSecs := metricsDao.PendingTasksOldestTask(ctx)
	c.metrics.TaskStats.With(prometheus.Labels{"label": instrumentation.TaskStatsLabelOldestWait}).Set(oldestQueuedSecs)

	err := c.pulpClient.Livez(ctx)
	if err != nil {
		c.metrics.PulpConnectivity.Set(0)
	} else {
		c.metrics.PulpConnectivity.Set(1)
	}

	taskPendingTimeAverageByType := metricsDao.TaskPendingTimeAverageByType(ctx)
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

	templatesUseLatestCount := metricsDao.TemplatesUseLatestCount(ctx)
	c.metrics.TemplatesUseLatestCount.Set(float64(templatesUseLatestCount))
	templatesUseDateCount := metricsDao.TemplatesUseDateCount(ctx)
	c.metrics.TemplatesUseDateCount.Set(float64(templatesUseDateCount))
	templatesCount := templatesUseLatestCount + templatesUseDateCount
	c.metrics.TemplatesCount.Set(float64(templatesCount))
	templatesUpdatedCount := metricsDao.TemplatesUpdatedInLast24HoursCount(ctx)
	c.metrics.TemplatesUpdatedInLast24HoursCount.Set(float64(templatesUpdatedCount))
	templatesAgeAverage := metricsDao.TemplatesAgeAverage(ctx)
	c.metrics.TemplatesAgeAverage.Set(templatesAgeAverage)

	if config.Get().Clients.PulpLogParser.Cloudwatch.Key != "" {
		date, err := c.dao.Memo.GetLastSuccessfulPulpLogDate(ctx)
		if err != nil {
			log.Error().Err(err).Msg("failed to read pulp last successful pulp log")
		}
		diff := time.Since(date)
		days := int(diff.Hours() / 24)
		c.metrics.PulpTransformLogsDaysSinceSuccess.Set(float64(days))
	}
}

func (c *Collector) snapshottingFailCheckIterate() {
	ctx := c.context
	c.metrics.RHReposSnapshotNotCompletedInLast36HoursCount.Set(float64(c.dao.Metrics.RHReposSnapshotNotCompletedInLast36HoursCount(ctx)))
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
