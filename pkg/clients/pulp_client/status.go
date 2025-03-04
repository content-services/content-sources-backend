package pulp_client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/content-services/content-sources-backend/pkg/cache"
	zest "github.com/content-services/zest/release/v2024"
	"github.com/rs/zerolog"
)

func (r *pulpDaoImpl) Status(ctx context.Context) (*zest.StatusResponse, error) {
	ctx, client := getZestClient(ctx)
	// Change this back to StatusRead(r.ctx) on next zest update
	status, resp, err := client.StatusAPI.StatusRead(ctx).Execute()
	if resp != nil {
		defer resp.Body.Close()
	}

	if err != nil {
		return nil, err
	}

	return status, nil
}

func (r *pulpDaoImpl) GetContentPath(ctx context.Context) (string, error) {
	logger := zerolog.Ctx(ctx)

	pulpContentPath, err := r.cache.GetPulpContentPath(ctx)
	if err != nil && !(errors.Is(err, cache.NotFound) || errors.Is(err, context.Canceled)) {
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

	pulpContentPath, err = url.JoinPath(contentOrigin, contentPathPrefix)
	if err != nil {
		return "", err
	}

	err = r.cache.SetPulpContentPath(ctx, pulpContentPath)
	if err != nil {
		logger.Error().Err(err).Msg("GetContentPath: error writing to cache")
	}

	return pulpContentPath, nil
}

func (r *pulpDaoImpl) Livez(ctx context.Context) error {
	ctx, client := getZestClient(ctx)

	resp, err := client.LivezAPI.LivezRead(ctx).Execute()
	if resp != nil {
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("livez check failed, error: %s", resp.Status)
		}
	}
	if err != nil {
		return err
	}

	return nil
}
