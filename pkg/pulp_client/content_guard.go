package pulp_client

import (
	"context"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	zest "github.com/content-services/zest/release/v2024"
	"github.com/openlyinc/pointy"
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
	return pulpHref, err
}

func (r pulpDaoImpl) createRHIDHeaderGuard(ctx context.Context, name string, jqFilter string, value string) (string, error) {
	ctx, client := getZestClient(ctx)
	guard := zest.HeaderContentGuard{
		Name:        name,
		Description: zest.NullableString{},
		HeaderName:  api.IdentityHeader,
		HeaderValue: value,
		JqFilter:    *zest.NewNullableString(pointy.Pointer(jqFilter)),
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

func (r pulpDaoImpl) fetchAndUpdateHeaderGuard(ctx context.Context, name string, jqFilter string, value string) (string, error) {
	ctx, client := getZestClient(ctx)

	resp, httpResp, err := client.ContentguardsHeaderAPI.ContentguardsCoreHeaderList(ctx, r.domainName).Name(name).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return "", errorWithResponseBody("error updating header guard", httpResp, err)
	}

	if resp.Count == nil || *resp.Count == 0 || resp.Results[0].PulpHref == nil {
		return "", nil
	}
	guard := resp.Results[0]
	if guard.HeaderName != api.IdentityHeader || guard.HeaderValue != value || guard.JqFilter.Get() == nil || *guard.JqFilter.Get() != jqFilter {
		update := zest.PatchedHeaderContentGuard{
			HeaderName:  pointy.Pointer(api.IdentityHeader),
			HeaderValue: &value,
			JqFilter:    *zest.NewNullableString(&jqFilter),
		}
		updateResp, updateHttpResp, err := client.ContentguardsHeaderAPI.ContentguardsCoreHeaderPartialUpdate(ctx, *guard.PulpHref).PatchedHeaderContentGuard(update).Execute()
		if updateHttpResp != nil {
			defer updateHttpResp.Body.Close()
		}
		if err != nil {
			return "", errorWithResponseBody("error updating header guard", httpResp, err)
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
	return pulpHref, err
}

func (r pulpDaoImpl) createCompositeGuard(ctx context.Context, guard1 string, guard2 string) (string, error) {
	ctx, client := getZestClient(ctx)

	guard := zest.CompositeContentGuard{
		Name:        COMPOSITE_GUARD_NAME,
		Description: zest.NullableString{},
		Guards:      []*string{pointy.Pointer(guard1), pointy.Pointer(guard2)},
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

func (r pulpDaoImpl) fetchOrUpdateCompositeGuard(ctx context.Context, guard1 string, guard2 string) (string, error) {
	ctx, client := getZestClient(ctx)
	resp, httpResp, err := client.ContentguardsCompositeAPI.ContentguardsCoreCompositeList(ctx, r.domainName).Name(COMPOSITE_GUARD_NAME).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return "", errorWithResponseBody("error listing composite guards", httpResp, err)
	}
	if resp.Count == nil || *resp.Count == 0 || resp.Results[0].PulpHref == nil {
		return "", nil
	}
	guard := resp.Results[0]
	if len(guard.Guards) != 2 || guard.Guards[0] == nil || *guard.Guards[0] != guard1 || guard.Guards[1] == nil || *guard.Guards[1] != guard2 {
		update := zest.PatchedCompositeContentGuard{
			Guards: []*string{pointy.Pointer(guard1), pointy.Pointer(guard2)},
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
