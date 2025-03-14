package pulp_client

import (
	"context"
	"fmt"
	"reflect"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/utils"
	zest "github.com/content-services/zest/release/v2025"
)

const ORG_ID_GUARD_NAME = "org_id_guard"
const ORG_ID_JQ_FILTER = ".identity.org_id"

const TURNPIKE_GUARD_NAME = "turnpike_guard"
const TURNPIKE_JQ_FILTER = ".identity.x509.subject_dn"

const COMPOSITE_GUARD_NAME = "composite_guard"

func (r pulpDaoImpl) CreateOrUpdateGuardsForOrg(ctx context.Context, orgId string) (string, error) {
	// First create/update/fetch the OrgId Guard
	OrgIdHref, err := r.CreateOrUpdateOrgIdGuard(ctx, orgId)
	if err != nil {
		return "", err
	}
	// Second create/update/fetch the guard for turnpike
	TurnpikeHref, err := r.CreateOrUpdateTurnpikeGuard(ctx)
	if err != nil {
		return "", err
	}

	// lastly join them together with the composite guard
	CompositeHref, err := r.createOrUpdateCompositeGuard(ctx, OrgIdHref, TurnpikeHref)
	return CompositeHref, err
}

func (r pulpDaoImpl) CreateOrUpdateOrgIdGuard(ctx context.Context, orgId string) (string, error) {
	return r.createOrUpdateRHIDHeaderGuard(ctx, ORG_ID_GUARD_NAME, ORG_ID_JQ_FILTER, orgId)
}

func (r pulpDaoImpl) CreateOrUpdateTurnpikeGuard(ctx context.Context) (string, error) {
	return r.createOrUpdateRHIDHeaderGuard(ctx, TURNPIKE_GUARD_NAME, TURNPIKE_JQ_FILTER, config.Get().Clients.Pulp.GuardSubjectDn)
}

func (r pulpDaoImpl) createOrUpdateRHIDHeaderGuard(ctx context.Context, name string, jqFilter string, value string) (string, error) {
	pulpHref, err := r.fetchAndUpdateHeaderGuard(ctx, name, jqFilter, value)
	if err != nil || pulpHref != "" {
		return pulpHref, err
	}
	// guard doesn't exist, so create it
	pulpHref, err = r.createRHIDHeaderGuard(ctx, name, jqFilter, value)
	if err != nil {
		guard, _ := r.fetchHeaderContentGuard(ctx, name)
		if guard == nil {
			return "", fmt.Errorf("failed to create and then fetch a RHID header %w", err)
		}
		return *guard.PulpHref, nil
	}
	return pulpHref, err
}

func (r pulpDaoImpl) createRHIDHeaderGuard(ctx context.Context, name string, jqFilter string, value string) (string, error) {
	ctx, client := getZestClient(ctx)
	guard := zest.HeaderContentGuard{
		Name:        name,
		Description: zest.NullableString{},
		HeaderName:  api.IdentityHeader,
		HeaderValue: value,
		JqFilter:    *zest.NewNullableString(utils.Ptr(jqFilter)),
	}

	response, httpResp, err := client.ContentguardsHeaderAPI.ContentguardsCoreHeaderCreate(ctx, r.domainName).
		HeaderContentGuard(guard).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return "", errorWithResponseBody("error creating header guard", httpResp, err)
	}
	return *response.PulpHref, nil
}

func (r pulpDaoImpl) fetchHeaderContentGuard(ctx context.Context, name string) (*zest.HeaderContentGuardResponse, error) {
	ctx, client := getZestClient(ctx)
	guard := zest.HeaderContentGuardResponse{}
	resp, httpResp, err := client.ContentguardsHeaderAPI.ContentguardsCoreHeaderList(ctx, r.domainName).Name(name).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return nil, errorWithResponseBody("error updating header guard", httpResp, err)
	}

	if resp.Count == 0 || resp.Results[0].PulpHref == nil {
		return nil, nil
	}
	guard = resp.Results[0]
	return &guard, nil
}

func (r pulpDaoImpl) fetchAndUpdateHeaderGuard(ctx context.Context, name string, jqFilter string, value string) (string, error) {
	ctx, client := getZestClient(ctx)
	guard, err := r.fetchHeaderContentGuard(ctx, name)
	if err != nil {
		return "", err
	} else if guard == nil {
		return "", nil
	}
	if guard.HeaderName != api.IdentityHeader || guard.HeaderValue != value || guard.JqFilter.Get() == nil || *guard.JqFilter.Get() != jqFilter {
		update := zest.PatchedHeaderContentGuard{
			HeaderName:  utils.Ptr(api.IdentityHeader),
			HeaderValue: &value,
			JqFilter:    *zest.NewNullableString(&jqFilter),
		}
		updateResp, updateHttpResp, err := client.ContentguardsHeaderAPI.ContentguardsCoreHeaderPartialUpdate(ctx, *guard.PulpHref).PatchedHeaderContentGuard(update).Execute()
		if updateHttpResp != nil {
			defer updateHttpResp.Body.Close()
		}
		if err != nil {
			return "", errorWithResponseBody("error updating header guard", updateHttpResp, err)
		}
		return *updateResp.PulpHref, nil
	}
	return *guard.PulpHref, nil
}

func (r pulpDaoImpl) createOrUpdateCompositeGuard(ctx context.Context, guard1href string, guard2href string) (string, error) {
	pulpHref, err := r.fetchOrUpdateCompositeGuard(ctx, guard1href, guard2href)
	if err != nil || pulpHref != "" {
		return pulpHref, err
	}
	// guard doesn't exist, so create it
	pulpHref, err = r.createCompositeGuard(ctx, guard1href, guard2href)
	if err != nil {
		guard, _ := r.fetchCompositeContentGuard(ctx)
		if guard == nil {
			return "", fmt.Errorf("failed to create and fetch composite content guard %w", err)
		}
		return *guard.PulpHref, nil
	}
	return pulpHref, err
}

func (r pulpDaoImpl) createCompositeGuard(ctx context.Context, guard1 string, guard2 string) (string, error) {
	ctx, client := getZestClient(ctx)

	guard := zest.CompositeContentGuard{
		Name:        COMPOSITE_GUARD_NAME,
		Description: zest.NullableString{},
		Guards:      []*string{utils.Ptr(guard1), utils.Ptr(guard2)},
	}
	response, httpResp, err := client.ContentguardsCompositeAPI.ContentguardsCoreCompositeCreate(ctx, r.domainName).
		CompositeContentGuard(guard).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return "", errorWithResponseBody("error creating composite guard", httpResp, err)
	}
	return *response.PulpHref, nil
}

func (r pulpDaoImpl) fetchCompositeContentGuard(ctx context.Context) (*zest.CompositeContentGuardResponse, error) {
	ctx, client := getZestClient(ctx)

	resp, httpResp, err := client.ContentguardsCompositeAPI.ContentguardsCoreCompositeList(ctx, r.domainName).Name(COMPOSITE_GUARD_NAME).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return nil, errorWithResponseBody("error listing composite guards", httpResp, err)
	}
	if resp.Count == 0 || resp.Results[0].PulpHref == nil {
		return nil, nil
	}
	guard := resp.Results[0]
	return &guard, nil
}

func (r pulpDaoImpl) fetchOrUpdateCompositeGuard(ctx context.Context, guard1 string, guard2 string) (string, error) {
	ctx, client := getZestClient(ctx)
	guard, err := r.fetchCompositeContentGuard(ctx)
	if err != nil {
		return "", err
	} else if guard == nil {
		return "", nil
	}
	if len(guard.Guards) != 2 || guard.Guards[0] == nil || *guard.Guards[0] != guard1 || guard.Guards[1] == nil || *guard.Guards[1] != guard2 {
		update := zest.PatchedCompositeContentGuard{
			Guards: []*string{utils.Ptr(guard1), utils.Ptr(guard2)},
		}
		updateResp, updateHttpResp, err := client.ContentguardsCompositeAPI.ContentguardsCoreCompositePartialUpdate(ctx, *guard.PulpHref).
			PatchedCompositeContentGuard(update).Execute()
		if updateHttpResp != nil {
			defer updateHttpResp.Body.Close()
		}
		if err != nil {
			return "", errorWithResponseBody("error updating composite guard", updateHttpResp, err)
		}
		return *updateResp.PulpHref, nil
	}
	return *guard.PulpHref, nil
}

func (r pulpDaoImpl) fetchFeatureGuard(ctx context.Context, featureName string) (*zest.ServiceFeatureContentGuardResponse, error) {
	ctx, client := getZestClient(ctx)
	resp, httpResp, err := client.ContentguardsFeatureAPI.ContentguardsServiceFeatureList(ctx, r.domainName).Name(featureGuardName(featureName)).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return nil, errorWithResponseBody("error listing feature guards", httpResp, err)
	}
	if resp.Count == 0 || resp.Results[0].PulpHref == nil {
		return nil, nil
	}
	guard := resp.Results[0]
	return &guard, nil
}

func featureGuardName(featureName string) string {
	return fmt.Sprintf("feature_%s", featureName)
}

func (r pulpDaoImpl) CreateOrUpdateFeatureGuard(ctx context.Context, featureName string) (string, error) {
	ctx, client := getZestClient(ctx)
	guard, err := r.fetchFeatureGuard(ctx, featureName)

	filter := zest.NullableString{}
	filter.Set(utils.Ptr(".identity.org_id"))

	guardToCreate := zest.ServiceFeatureContentGuard{
		Name:       featureGuardName(featureName),
		HeaderName: api.IdentityHeader,
		JqFilter:   filter,
		Features:   []string{featureName},
	}

	if err != nil {
		return "", err
	} else if guard != nil { // Already created check for differences
		if guardToCreate.HeaderName != guard.HeaderName || guardToCreate.JqFilter != guard.JqFilter || !reflect.DeepEqual(guardToCreate.Features, guard.Features) {
			resp, httpResp, err := client.ContentguardsFeatureAPI.ContentguardsServiceFeatureUpdate(ctx, *guard.PulpHref).ServiceFeatureContentGuard(guardToCreate).Execute()
			if httpResp != nil {
				defer httpResp.Body.Close()
				if err != nil {
					return "", errorWithResponseBody("error updating feature guard", httpResp, err)
				}
				return *resp.PulpHref, nil
			}
		}
		return *guard.PulpHref, nil
	} else { // create it
		resp, httpResp, err := client.ContentguardsFeatureAPI.ContentguardsServiceFeatureCreate(ctx, r.domainName).ServiceFeatureContentGuard(guardToCreate).Execute()
		if httpResp != nil {
			defer httpResp.Body.Close()
		}
		if err != nil {
			return "", errorWithResponseBody("error creating feature guard", httpResp, err)
		}
		return *resp.PulpHref, nil
	}
}
