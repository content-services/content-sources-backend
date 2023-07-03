package pulp_client

import zest "github.com/content-services/zest/release/v2023"

// CreateRpmPublication Creates a Publication
func (r *pulpDaoImpl) CreateRpmPublication(versionHref string) (*string, error) {
	rpmRpmRepository := *zest.NewRpmRpmPublication()
	rpmRpmRepository.RepositoryVersion = &versionHref
	resp, httpResp, err := r.client.PublicationsRpmAPI.PublicationsRpmRpmCreate(r.ctx, r.domainName).RpmRpmPublication(rpmRpmRepository).Execute()

	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()

	taskHref := resp.GetTask()
	return &taskHref, nil
}

func (r *pulpDaoImpl) FindRpmPublicationByVersion(versionHref string) (*zest.RpmRpmPublicationResponse, error) {
	resp, httpResp, err := r.client.PublicationsRpmAPI.PublicationsRpmRpmList(r.ctx, r.domainName).RepositoryVersion(versionHref).Execute()
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()

	results := resp.GetResults()
	if len(results) > 0 {
		return &results[0], nil
	} else {
		return nil, nil
	}
}
