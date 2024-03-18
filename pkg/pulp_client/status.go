package pulp_client

import (
	"context"
	"errors"

	"github.com/content-services/content-sources-backend/pkg/cache"
	zest "github.com/content-services/zest/release/v2024"
	"github.com/rs/zerolog"
)

func (r *pulpDaoImpl) Status(ctx context.Context) (*zest.StatusResponse, error) {
	ctx, client := getZestClient(ctx)
	// Change this back to StatusRead(r.ctx) on next zest update
	status, resp, err := client.StatusAPI.StatusRead(ctx).Execute()
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return status, nil
}

func (r *pulpDaoImpl) GetContentPath(ctx context.Context) (string, error) {
	logger := zerolog.Ctx(ctx)

	pulpContentPath, err := r.cache.GetPulpContentPath(ctx)
	if err != nil && !errors.Is(err, cache.NotFound) {
		logger.Error().Err(err).Msg("GetContentPath: error reading from cache")
	}

	cacheHit := err == nil
	if cacheHit {
		return pulpContentPath, nil
	}

	resp, err := r.Status(ctx)
	if err != nil {
		return "", err
	}

	contentOrigin := resp.ContentSettings.ContentOrigin
	contentPathPrefix := resp.ContentSettings.ContentPathPrefix
	pulpContentPath = contentOrigin + contentPathPrefix

	err = r.cache.SetPulpContentPath(ctx, pulpContentPath)
	if err != nil {
		logger.Error().Err(err).Msg("GetContentPath: error writing to cache")
	}

	return contentOrigin + contentPathPrefix, nil
}
