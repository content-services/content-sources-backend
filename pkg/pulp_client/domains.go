package pulp_client

import (
	"fmt"

	"github.com/content-services/content-sources-backend/pkg/config"
	zest "github.com/content-services/zest/release/v2023"
)

const DefaultDomain = "default"

func S3StorageConfiguration() map[string]interface{} {
	loaded := config.Get().Clients.Pulp.CustomRepoObjects
	s3Config := make(map[string]interface{})

	s3Config["aws_default_acl"] = "@none None"
	s3Config["aws_s3_region_name"] = loaded.Region
	s3Config["endpoint_url"] = loaded.URL
	s3Config["aws_access_key_id"] = loaded.AccessKey
	s3Config["secret_key"] = loaded.SecretKey
	s3Config["aws_storage_bucket_name"] = loaded.Name
	return s3Config
}

func (r *pulpDaoImpl) LookupOrCreateDomain(name string) (*string, error) {
	href, err := r.LookupDomain(name)
	if err != nil {
		return nil, err
	}
	if href != nil {
		return href, nil
	}
	href, createErr := r.CreateDomain(name)
	if err == nil {
		return href, nil
	} else {
		// If we get an error, lookup the domain again to see if another request created it
		//  if its still not there, return the create error
		href, err := r.LookupDomain(name)
		if href == nil || err != nil {
			return nil, createErr
		} else {
			return href, nil
		}
	}
}

func (r *pulpDaoImpl) LookupDomain(name string) (*string, error) {
	list, resp, err := r.client.DomainsAPI.DomainsList(r.ctx, "default").Name(name).Execute()
	defer resp.Body.Close()
	if err != nil {
		return nil, err
	}
	if len(list.Results) == 0 {
		return nil, nil
	} else {
		return list.Results[0].PulpHref, nil
	}
}

// CreateRpmPublication Creates a Publication
func (r *pulpDaoImpl) CreateDomain(name string) (*string, error) {
	s3Storage := zest.STORAGECLASSENUM_STORAGES_BACKENDS_S3BOTO3_S3_BOTO3_STORAGE
	localStorage := zest.STORAGECLASSENUM_PULPCORE_APP_MODELS_STORAGE_FILE_SYSTEM
	var domain zest.Domain
	if config.Get().Clients.Pulp.StorageType == config.STORAGE_TYPE_OBJECT {
		domain = *zest.NewDomain(name, s3Storage, S3StorageConfiguration())
	} else {
		emptyConfig := make(map[string]interface{})
		emptyConfig["location"] = fmt.Sprintf("/var/lib/pulp/%v/", name)
		domain = *zest.NewDomain(name, localStorage, emptyConfig)
	}
	domainResp, resp, err := r.client.DomainsAPI.DomainsCreate(r.ctx, DefaultDomain).Domain(domain).Execute()
	if err != nil {
		return nil, err
	}
	if resp.Body != nil {
		defer resp.Body.Close()
	}
	return domainResp.PulpHref, nil
}
