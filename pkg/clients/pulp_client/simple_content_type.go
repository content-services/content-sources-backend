package pulp_client

import (
	"context"
	"errors"

	"github.com/content-services/content-sources-backend/pkg/cache"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/rs/zerolog/log"
)

func (p *pulpDaoImpl) pulpSimpleContentForType(contentType string) PulpSimpleContentType {
	var pc PulpClient = p
	if contentType == "maven" { // TODO don't hardcode this
		return NewMavenSimpleContentType(&pc)
	}
	return nil
}

func (p *pulpDaoImpl) SimplePackageCount(ctx context.Context, repoContentType string, repositoryName string) (packageCount int64, cacheUpdated bool, err error) {
	summary, cacheUpdated, err := p.ContentSummary(ctx, repoContentType, repositoryName)
	if err != nil || summary == nil {
		return 0, false, err
	}
	packageContentType := p.pulpSimpleContentForType(repoContentType).PrimaryPackageTypeLabel()
	value := (*summary)[packageContentType]

	return value, cacheUpdated, err
}

func (p *pulpDaoImpl) ContentSummary(ctx context.Context, contentType string, repositoryName string) (*models.ContentCountsType, bool, error) {
	cachedCounts, err := p.cache.GetContentCounts(ctx, p.domainName, repositoryName)

	if err != nil && !errors.Is(err, cache.ErrNotFound) {
		log.Error().Err(err).Msg("Content counts - error reading from cache")
	}
	if cachedCounts != nil {
		return cachedCounts, false, nil
	}
	zestCounts, err := p.pulpSimpleContentForType(contentType).FetchLatestContentCounts(ctx, repositoryName)
	if err != nil {
		return nil, false, err
	}
	counts, _, _ := models.ContentSummaryToContentCounts(zestCounts)
	err = p.cache.SetContentCounts(ctx, p.domainName, repositoryName, counts)
	if err != nil {
		log.Error().Err(err).Msg("Content counts - error updating cache")
	}
	return &counts, true, nil
}
