package candlepin_client

import (
	"context"
	"net/http"

	caliri "github.com/content-services/caliri/release/v4"
	"github.com/content-services/content-sources-backend/pkg/utils"
)

func (c *cpClientImpl) CreateConsumer(ctx context.Context, orgID string, name string) (*caliri.ConsumerDTO, error) {
	ctx, client, err := getCandlepinClient(ctx)
	if err != nil {
		return nil, err
	}

	consumerDTO := caliri.ConsumerDTO{
		Name: utils.Ptr(name),
		Type: &caliri.ConsumerTypeDTO{
			Label: utils.Ptr("system"),
		},
	}
	consumer, httpResp, err := client.ConsumerAPI.CreateConsumer(ctx).Owner(OwnerKey(orgID)).ConsumerDTO(consumerDTO).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return nil, errorWithResponseBody("couldn't create consumer", httpResp, err)
	}
	return consumer, nil
}

func (c *cpClientImpl) FetchConsumer(ctx context.Context, consumerUUID string) (*caliri.ConsumerDTO, error) {
	ctx, client, err := getCandlepinClient(ctx)
	if err != nil {
		return nil, err
	}

	consumer, httpResp, err := client.ConsumerAPI.GetConsumer(ctx, consumerUUID).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
			return nil, nil
		}
		return nil, errorWithResponseBody("couldn't list consumers", httpResp, err)
	}

	return consumer, nil
}

func (c *cpClientImpl) DeleteConsumer(ctx context.Context, consumerUUID string) error {
	ctx, client, err := getCandlepinClient(ctx)
	if err != nil {
		return err
	}

	httpResp, err := client.ConsumerAPI.DeleteConsumer(ctx, consumerUUID).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		if httpResp != nil && (httpResp.StatusCode == http.StatusNotFound || httpResp.StatusCode == http.StatusGone) {
			return nil
		}
		return errorWithResponseBody("couldn't delete consumer", httpResp, err)
	}
	return nil
}
