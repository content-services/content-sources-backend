package candlepin_client

import "fmt"

func (c *cpClientImpl) ListContents(ownerKey string) ([]string, error) {
	labels := []string{}
	contents, httpResp, err := c.client.OwnerContentAPI.GetContentsByOwner(c.ctx, ownerKey).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return labels, fmt.Errorf("could not fetch contents for owner %w", err)
	}

	for _, c := range contents {
		labels = append(labels, *c.Label)
	}
	return labels, nil
}
