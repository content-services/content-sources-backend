package pulp_client

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/utils"
	zest "github.com/content-services/zest/release/v2026"
)

const ORG_ID_GUARD_NAME = "org_id_guard"
const ORG_ID_JQ_FILTER = ".identity.org_id"

const TURNPIKE_GUARD_NAME = "turnpike_guard"
const TURNPIKE_JQ_FILTER = ".identity.x509.subject_dn"

const COMPOSITE_GUARD_NAME = "composite_guard"

func rhelCompositeGuardName(features []string) string {
	return "rhel_composite_" + strings.Join(features, "_")
}

func featureGuardPulpName(features []string) string {
	return "feature_" + strings.Join(features, "_")
}

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

func (r pulpDaoImpl) CreateOrUpdateGuardsForRhelRepo(ctx context.Context, featureName string) (string, error) {
	features := utils.ParseFeatures(featureName)
	if len(features) == 0 {
		return "", fmt.Errorf("feature name required for RHEL composite content guard")
	}
	guardHrefs := make([]string, 0, len(features)+1)
	for _, f := range features {
		href, err := r.CreateOrUpdateFeatureGuard(ctx, f)
		if err != nil {
			return "", err
		}
		guardHrefs = append(guardHrefs, href)
	}
	turnpikeHref, err := r.CreateOrUpdateTurnpikeGuard(ctx)
	if err != nil {
		return "", err
	}
	guardHrefs = append(guardHrefs, turnpikeHref)
	return r.createOrUpdateRhelCompositeGuard(ctx, guardHrefs, features)
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
	ctx, client, err := getZestClient(ctx)
	if err != nil {
		return "", err
	}
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
	ctx, client, err := getZestClient(ctx)
	if err != nil {
		return nil, err
	}
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
	ctx, client, err := getZestClient(ctx)
	if err != nil {
		return "", err
	}
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
	return r.createNamedCompositeGuard(ctx, COMPOSITE_GUARD_NAME, []string{guard1, guard2})
}

func (r pulpDaoImpl) fetchCompositeContentGuard(ctx context.Context) (*zest.CompositeContentGuardResponse, error) {
	return r.fetchNamedCompositeContentGuard(ctx, COMPOSITE_GUARD_NAME)
}

func (r pulpDaoImpl) fetchOrUpdateCompositeGuard(ctx context.Context, guard1 string, guard2 string) (string, error) {
	return r.fetchOrUpdateNamedCompositeGuard(ctx, COMPOSITE_GUARD_NAME, []string{guard1, guard2})
}

func ptrStringsFromHrefs(hrefs []string) []*string {
	out := make([]*string, len(hrefs))
	for i := range hrefs {
		out[i] = utils.Ptr(hrefs[i])
	}
	return out
}

func compositeChildHrefsEqual(existing []*string, want []string) bool {
	if len(existing) != len(want) {
		return false
	}
	for i, w := range want {
		if existing[i] == nil || *existing[i] != w {
			return false
		}
	}
	return true
}

func (r pulpDaoImpl) fetchNamedCompositeContentGuard(ctx context.Context, name string) (*zest.CompositeContentGuardResponse, error) {
	ctx, client, err := getZestClient(ctx)
	if err != nil {
		return nil, err
	}

	resp, httpResp, err := client.ContentguardsCompositeAPI.ContentguardsCoreCompositeList(ctx, r.domainName).Name(name).Execute()
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

func (r pulpDaoImpl) createNamedCompositeGuard(ctx context.Context, name string, guardHrefs []string) (string, error) {
	ctx, client, err := getZestClient(ctx)
	if err != nil {
		return "", err
	}

	guard := zest.CompositeContentGuard{
		Name:        name,
		Description: zest.NullableString{},
		Guards:      ptrStringsFromHrefs(guardHrefs),
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

func (r pulpDaoImpl) fetchOrUpdateNamedCompositeGuard(ctx context.Context, name string, guardHrefs []string) (string, error) {
	ctx, client, err := getZestClient(ctx)
	if err != nil {
		return "", err
	}
	guard, err := r.fetchNamedCompositeContentGuard(ctx, name)
	if err != nil {
		return "", err
	} else if guard == nil {
		return "", nil
	}
	if !compositeChildHrefsEqual(guard.Guards, guardHrefs) {
		update := zest.PatchedCompositeContentGuard{
			Guards: ptrStringsFromHrefs(guardHrefs),
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

func (r pulpDaoImpl) createOrUpdateNamedCompositeGuard(ctx context.Context, name string, guardHrefs []string) (string, error) {
	pulpHref, err := r.fetchOrUpdateNamedCompositeGuard(ctx, name, guardHrefs)
	if err != nil || pulpHref != "" {
		return pulpHref, err
	}
	pulpHref, err = r.createNamedCompositeGuard(ctx, name, guardHrefs)
	if err != nil {
		guard, _ := r.fetchNamedCompositeContentGuard(ctx, name)
		if guard == nil {
			return "", fmt.Errorf("failed to create and fetch composite content guard %q: %w", name, err)
		}
		return *guard.PulpHref, nil
	}
	return pulpHref, err
}

func (r pulpDaoImpl) createOrUpdateRhelCompositeGuard(ctx context.Context, guardHrefs []string, features []string) (string, error) {
	return r.createOrUpdateNamedCompositeGuard(ctx, rhelCompositeGuardName(features), guardHrefs)
}

func (r pulpDaoImpl) fetchFeatureGuard(ctx context.Context, features []string) (*zest.ServiceFeatureContentGuardResponse, error) {
	ctx, client, err := getZestClient(ctx)
	if err != nil {
		return nil, err
	}
	resp, httpResp, err := client.ContentguardsFeatureAPI.ContentguardsServiceFeatureList(ctx, r.domainName).Name(featureGuardPulpName(features)).Execute()
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

// CreateOrUpdateFeatureGuard creates or updates one Pulp ServiceFeatureContentGuard for a single feature.
// Pass one feature token (no comma-separated list); use CreateOrUpdateGuardsForRhelRepo for multiple features.
// A ServiceFeatureContentGuard with multiple Features uses AND semantics in Pulp, so this API only ever sets one feature per guard.
func (r pulpDaoImpl) CreateOrUpdateFeatureGuard(ctx context.Context, featureName string) (string, error) {
	features := utils.ParseFeatures(featureName)
	if len(features) == 0 {
		return "", fmt.Errorf("empty feature name for content guard")
	}
	if len(features) != 1 {
		return "", fmt.Errorf("CreateOrUpdateFeatureGuard expects a single feature name, got %d after parsing", len(features))
	}
	featureSlice := features
	ctx, client, err := getZestClient(ctx)
	if err != nil {
		return "", err
	}
	guard, err := r.fetchFeatureGuard(ctx, featureSlice)

	filter := zest.NullableString{}
	filter.Set(utils.Ptr(".identity.org_id"))

	guardToCreate := zest.ServiceFeatureContentGuard{
		Name:       featureGuardPulpName(featureSlice),
		HeaderName: api.IdentityHeader,
		JqFilter:   filter,
		Features:   featureSlice,
	}

	if err != nil {
		return "", err
	} else if guard != nil { // Already created check for differences
		guardFeat := append([]string(nil), guard.Features...)
		slices.Sort(guardFeat)
		wantFeat := append([]string(nil), guardToCreate.Features...)
		slices.Sort(wantFeat)
		if guardToCreate.HeaderName != guard.HeaderName || guardToCreate.JqFilter != guard.JqFilter || !slices.Equal(wantFeat, guardFeat) {
			resp, httpResp, err := client.ContentguardsFeatureAPI.ContentguardsServiceFeatureUpdate(ctx, *guard.PulpHref).ServiceFeatureContentGuard(guardToCreate).Execute()
			if httpResp != nil {
				defer httpResp.Body.Close()
			}
			if err != nil {
				return "", errorWithResponseBody("error updating feature guard", httpResp, err)
			}
			return *resp.PulpHref, nil
		}
		return *guard.PulpHref, nil
	}
	resp, httpResp, err := client.ContentguardsFeatureAPI.ContentguardsServiceFeatureCreate(ctx, r.domainName).ServiceFeatureContentGuard(guardToCreate).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return "", errorWithResponseBody("error creating feature guard", httpResp, err)
	}
	return *resp.PulpHref, nil
}
