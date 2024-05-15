package candlepin_client

import (
	"context"
	"fmt"
	"os"

	caliri "github.com/content-services/caliri/release/v4"
)

func (c *cpClientImpl) ImportManifest(ctx context.Context, filename string) error {
	ctx, client, err := getCandlepinClient(ctx)
	if err != nil {
		return err
	}

	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("Could not open manifest %w", err)
	}
	defer file.Close()
	asyncTask, httpResp, err := client.OwnerAPI.ImportManifestAsync(ctx, DevelOrgKey).Force([]string{"SIGNATURE_CONFLICT"}).Input(file).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return errorWithResponseBody("couldn't import manifest", httpResp, err)
	}

	for asyncTask.EndTime == nil {
		asyncTask, err = c.pollTask(ctx, *asyncTask)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *cpClientImpl) pollTask(ctx context.Context, asyncTask caliri.AsyncJobStatusDTO) (*caliri.AsyncJobStatusDTO, error) {
	ctx, client, err := getCandlepinClient(ctx)
	if err != nil {
		return nil, err
	}

	async, httpResp, err := client.JobsAPI.GetJobStatus(ctx, *asyncTask.Id).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return async, errorWithResponseBody("couldn't fetch async job", httpResp, err)
	}
	return async, nil
}
