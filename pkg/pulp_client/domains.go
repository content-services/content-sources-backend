package pulp_client

import (
	"bytes"
	"fmt"
	"reflect"

	"github.com/content-services/content-sources-backend/pkg/config"
	zest "github.com/content-services/zest/release/v2023"
	"github.com/rs/zerolog/log"
)

const DefaultDomain = "default"

func S3StorageConfiguration() map[string]interface{} {
	loaded := config.Get().Clients.Pulp.CustomRepoObjects
	s3Config := make(map[string]interface{})

	s3Config["aws_default_acl"] = "private"
	s3Config["aws_s3_region_name"] = loaded.Region
	s3Config["endpoint_url"] = loaded.URL
	s3Config["access_key"] = loaded.AccessKey
	s3Config["secret_key"] = loaded.SecretKey
	s3Config["aws_storage_bucket_name"] = loaded.Name
	return s3Config
}

func (r *pulpDaoImpl) LookupOrCreateDomain(name string) (string, error) {
	href, err := r.LookupDomain(name)
	if err != nil {
		return "", err
	}
	if href != "" {
		return href, nil
	}
	href, createErr := r.CreateDomain(name)
	if err == nil {
		return href, nil
	} else {
		// If we get an error, lookup the domain again to see if another request created it
		//  if its still not there, return the create error
		href, err := r.LookupDomain(name)
		if href == "" || err != nil {
			return "", createErr
		} else {
			return href, nil
		}
	}
}

// Updates a domain if that domain is using s3 and its storage configuration has changed
func (r *pulpDaoImpl) UpdateDomainIfNeeded(name string) error {
	if config.Get().Clients.Pulp.StorageType == config.STORAGE_TYPE_LOCAL {
		return nil
	}
	domain, err := r.lookupDomain(name)
	if err != nil {
		return err
	}
	expectedConfig := S3StorageConfiguration()
	if !reflect.DeepEqual(domain.StorageSettings, expectedConfig) {
		patchedDomain := zest.PatchedDomain{
			StorageSettings: S3StorageConfiguration(),
		}
		_, resp, err := r.client.DomainsAPI.DomainsPartialUpdate(r.ctx, *domain.PulpHref).PatchedDomain(patchedDomain).Execute()
		if resp.Body != nil {
			defer resp.Body.Close()
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *pulpDaoImpl) lookupDomain(name string) (*zest.DomainResponse, error) {
	list, resp, err := r.client.DomainsAPI.DomainsList(r.ctx, "default").Name(name).Execute()
	defer resp.Body.Close()
	if err != nil {
		return nil, err
	}
	if len(list.Results) == 0 {
		return nil, nil
	} else if list.Results[0].PulpHref == nil {
		return nil, fmt.Errorf("Unexpectedly got a nil href for domain %v", name)
	} else {
		return &list.Results[0], nil
	}
}

func (r *pulpDaoImpl) LookupDomain(name string) (string, error) {
	d, err := r.lookupDomain(name)
	if err != nil || d == nil || d.PulpHref == nil {
		return "", err
	}
	return *d.PulpHref, nil
}

// CreateRpmPublication Creates a Publication
func (r *pulpDaoImpl) CreateDomain(name string) (string, error) {
	s3Storage := zest.STORAGECLASSENUM_STORAGES_BACKENDS_S3BOTO3_S3_BOTO3_STORAGE
	localStorage := zest.STORAGECLASSENUM_PULPCORE_APP_MODELS_STORAGE_FILE_SYSTEM
	var domain zest.Domain
	if config.Get().Clients.Pulp.StorageType == config.STORAGE_TYPE_OBJECT {
		config := S3StorageConfiguration()
		domain = *zest.NewDomain(name, s3Storage, config)
	} else {
		emptyConfig := make(map[string]interface{})
		emptyConfig["location"] = fmt.Sprintf("/var/lib/pulp/%v/", name)
		domain = *zest.NewDomain(name, localStorage, emptyConfig)
	}
	domainResp, resp, err := r.client.DomainsAPI.DomainsCreate(r.ctx, DefaultDomain).Domain(domain).Execute()
	if resp.Body != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		body := ""
		if resp.Body != nil {
			buf := new(bytes.Buffer)
			_, err := buf.ReadFrom(resp.Body)
			if err == nil {
				body = buf.String()
			} else {
				log.Error().Err(err).Msg("Error reading body from failed domain creation.")
			}
		}
		log.Warn().Err(err).Str("body", body).Msg("Error creating domain")
		return "", err
	}
	return *domainResp.PulpHref, nil
}
