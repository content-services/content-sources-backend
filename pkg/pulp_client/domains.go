package pulp_client

import (
	"encoding/json"
	"fmt"

	"github.com/content-services/content-sources-backend/pkg/config"
	zest "github.com/content-services/zest/release/v2023"
)

func S3StorageConfiguration() map[string]interface{} {
	loaded := config.Get().Clients.Pulp.CustomRepoObjects
	s3Config := make(map[string]interface{})

	s3Config["aws_default_acl"] = "@none None"
	s3Config["aws_s3_region_name"] = loaded.Region
	s3Config["aws_s3_endpoint_url"] = loaded.URL
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
	href, err = r.CreateDomain(name)
	if err == nil {
		return href, nil
	} else {
		isDupl, err := processError(err)
		if isDupl {
			return r.LookupDomain(name)
		} else {
			return nil, err
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

	domainResp, resp, err := r.client.DomainsAPI.DomainsCreate(r.ctx, "default").Domain(domain).Execute()
	defer resp.Body.Close()
	if err != nil {
		return nil, err
	}
	return domainResp.PulpHref, nil
}

// processError checks to see if the error is a duplicate name error
//
//	and if not, tries to combine the body text of the response in a single error message
func processError(apiError error) (bool, error) {
	zestError, ok := apiError.(*zest.GenericOpenAPIError)
	if !ok {
		return false, apiError
	}
	parsed := make(map[string][]string)
	err := json.Unmarshal(zestError.Body(), &parsed)
	betterErr := fmt.Errorf("%v: %v", err.Error(), string(zestError.Body()))
	if err != nil {
		return false, betterErr
	}
	for _, message := range parsed["name"] {
		if message == "This field must be unique." {
			return true, nil
		}
	}
	return false, betterErr
}
