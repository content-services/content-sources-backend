package candlepin_client

import (
	"fmt"
	"os"

	caliri "github.com/content-services/caliri/release/v4"
)

func (c *cpClientImpl) ImportManifest(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("Could not open manifest %w", err)
	}
	defer file.Close()
	asyncTask, httpResp, err := c.client.OwnerAPI.ImportManifestAsync(c.ctx, DevelOrgKey).Force([]string{"SIGNATURE_CONFLICT"}).Input(file).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return errorWithResponseBody("couldn't import manifest", httpResp, err)
	}

	for asyncTask.EndTime == nil {
		asyncTask, err = c.pollTask(*asyncTask)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *cpClientImpl) pollTask(asyncTask caliri.AsyncJobStatusDTO) (*caliri.AsyncJobStatusDTO, error) {
	async, httpResp, err := c.client.JobsAPI.GetJobStatus(c.ctx, *asyncTask.Id).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return async, errorWithResponseBody("couldn't fetch async job	", httpResp, err)
	}
	return async, nil
}
