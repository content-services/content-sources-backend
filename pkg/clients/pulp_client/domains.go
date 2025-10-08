package pulp_client

import (
	"context"
	"fmt"
	"reflect"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/utils"
	zest "github.com/content-services/zest/release/v2025"
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

func (r *pulpDaoImpl) LookupOrCreateDomain(ctx context.Context, name string) (string, error) {
	href, err := r.LookupDomain(ctx, name)
	if err != nil {
		return "", err
	}
	if href != "" {
		return href, nil
	}
	href, createErr := r.CreateDomain(ctx, name)
	if createErr == nil {
		return href, nil
	} else {
		// If we get an error, lookup the domain again to see if another request created it
		//  if its still not there, return the create error
		href, err := r.LookupDomain(ctx, name)
		if href == "" || err != nil {
			return "", createErr
		} else {
			return href, nil
		}
	}
}

func (r *pulpDaoImpl) UpdateDomainIfNeeded(ctx context.Context, name string) error {
	// Updates a domain if that domain is using s3 and its storage configuration has changed
	ctx, client, err := getZestClient(ctx)
	if err != nil {
		return err
	}
	if config.Get().Clients.Pulp.StorageType == config.STORAGE_TYPE_LOCAL {
		return nil
	}
	domain, err := r.lookupDomain(ctx, name)
	if err != nil {
		return err
	}
	expectedConfig := S3StorageConfiguration()
	if !reflect.DeepEqual(domain.StorageSettings, expectedConfig) {
		patchedDomain := zest.PatchedDomain{
			StorageSettings: S3StorageConfiguration(),
		}

		// Execute returns as the first parameter either no taskHref and resp.StatusCode 200 or taskHref with resp.StatusCode 202
		_, resp, err := client.DomainsAPI.DomainsPartialUpdate(ctx, *domain.PulpHref).PatchedDomain(patchedDomain).Execute()
		if resp != nil && resp.Body != nil {
			defer resp.Body.Close()
		}

		// errorMsg is temporary workaround (zest throws error since it expects a pulpTaskHref to be always returned)
		// until zest gets updated upon pulp update
		var errorMsg string
		if err != nil {
			errorMsg = err.Error()
		}
		if err != nil && errorMsg != "no value given for required property task" {
			return errorWithResponseBody("error updating domain", resp, err)
		}
	}
	return nil
}

func (r *pulpDaoImpl) SetDomainLabel(ctx context.Context, pulpHref string, key, value string) error {
	ctx, client, err := getZestClient(ctx)
	if err != nil {
		return err
	}
	_, resp, err := client.DomainsAPI.
		DomainsSetLabel(ctx, pulpHref).
		SetLabel(*zest.NewSetLabel(key, *zest.NewNullableString(utils.Ptr(value)))).
		Execute()
	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return errorWithResponseBody("error updating domain label", resp, err)
	}
	return nil
}

func (r *pulpDaoImpl) lookupDomain(ctx context.Context, name string) (*zest.DomainResponse, error) {
	ctx, client, err := getZestClient(ctx)
	if err != nil {
		return nil, err
	}
	list, resp, err := client.DomainsAPI.DomainsList(ctx, "default").Name(name).Execute()
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return nil, errorWithResponseBody("error listing domains", resp, err)
	}
	if len(list.Results) == 0 {
		return nil, nil
	} else if list.Results[0].PulpHref == nil {
		return nil, fmt.Errorf("unexpectedly got a nil href for domain %v", name)
	} else {
		return &list.Results[0], nil
	}
}

func (r *pulpDaoImpl) LookupDomain(ctx context.Context, name string) (string, error) {
	d, err := r.lookupDomain(ctx, name)
	if err != nil || d == nil || d.PulpHref == nil {
		return "", err
	}
	return *d.PulpHref, nil
}

// CreateRpmPublication Creates a Publication
func (r *pulpDaoImpl) CreateDomain(ctx context.Context, name string) (string, error) {
	ctx, client, err := getZestClient(ctx)
	if err != nil {
		return "", err
	}

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
	domain.SetPulpLabels(map[string]string{"contentsources": "true"})
	domainResp, resp, err := client.DomainsAPI.DomainsCreate(ctx, DefaultDomain).Domain(domain).Execute()
	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return "", errorWithResponseBody("error creating domain", resp, err)
	}
	return *domainResp.PulpHref, nil
}
