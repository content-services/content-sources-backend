package pulp_client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/content-services/content-sources-backend/pkg/config"
	zest "github.com/content-services/zest/release/v2025"
)

func (r *pulpDaoImpl) Status(ctx context.Context) (*zest.StatusResponse, error) {
	ctx, client, err := getZestClient(ctx)
	if err != nil {
		return nil, err
	}
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
	resp, err := r.Status(ctx)
	if err != nil {
		return "", err
	}

	contentPathPrefix := resp.ContentSettings.ContentPathPrefix

	pulpContentPath, err := url.JoinPath(config.Get().Clients.Pulp.ContentOrigin, contentPathPrefix)
	if err != nil {
		return "", err
	}

	return pulpContentPath, nil
}

func (r *pulpDaoImpl) Livez(ctx context.Context) error {
	ctx, client, err := getZestClient(ctx)
	if err != nil {
		return err
	}

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
