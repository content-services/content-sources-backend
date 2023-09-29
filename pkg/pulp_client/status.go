package pulp_client

import (
	"path/filepath"

	zest "github.com/content-services/zest/release/v2023"
)

func (r *pulpDaoImpl) Status() (*zest.StatusResponse, error) {
	// Change this back to StatusRead(r.ctx) on next zest update
	status, resp, err := r.client.StatusAPI.StatusRead(r.ctx).Execute()
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return status, nil
}

// TODO move this to a better place
func (r *pulpDaoImpl) GetContentURL(distPath string) (string, error) {
	resp, err := r.Status()
	if err != nil {
		return "", err
	}
	contentOrigin := resp.ContentSettings.ContentOrigin
	contentPathPrefix := resp.ContentSettings.ContentPathPrefix

	url := filepath.Join(contentOrigin, contentPathPrefix, r.domainName, distPath)
	return url, nil
}
