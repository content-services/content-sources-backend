package pulp_client

import zest "github.com/content-services/zest/release/v3"

// CreateRpmPublication Creates a Publication
func (r pulpDaoImpl) CreateRpmPublication(versionHref string) (*string, error) {
	rpmRpmRepository := *zest.NewRpmRpmPublication()
	rpmRpmRepository.RepositoryVersion = &versionHref
	resp, _, err := r.client.PublicationsRpmApi.PublicationsRpmRpmCreate(r.ctx).RpmRpmPublication(rpmRpmRepository).Execute()

	if err != nil {
		return nil, err
	}
	taskHref := resp.GetTask()
	return &taskHref, nil
}

func (r pulpDaoImpl) FindRpmPublicationByVersion(versionHref string) (*zest.RpmRpmPublicationResponse, error) {
	resp, _, err := r.client.PublicationsRpmApi.PublicationsRpmRpmList(r.ctx).RepositoryVersion(versionHref).Execute()
	if err != nil {
		return nil, err
	}

	results := resp.GetResults()
	if len(results) > 0 {
		return &results[0], nil
	} else {
		return nil, nil
	}
}
