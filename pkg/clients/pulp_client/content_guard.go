package pulp_client

import (
	"context"
	"encoding/json"
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

const IMAGE_BUILDER_GUARD_NAME = "image_builder_guard"

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
	// Third create/update/fetch the TBR guard for Image Builder
	imageBuilderHref, err := r.CreateOrUpdateImageBuilderGuard(ctx)
	if err != nil {
		return "", err
	}

	guardHrefs := []string{OrgIdHref, TurnpikeHref}
	if imageBuilderHref != "" {
		guardHrefs = append(guardHrefs, imageBuilderHref)
	}
	return r.createOrUpdateCompositeGuard(ctx, guardHrefs...)
}

func (r pulpDaoImpl) CreateOrUpdateGuardsForRhelRepo(ctx context.Context, featureName string) (string, error) {
	features := utils.ParseFeatures(featureName)
	if len(features) == 0 {
		return "", fmt.Errorf("feature name required for RHEL composite content guard")
	}
	guardHrefs := make([]string, 0, len(features)+2)
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
	// Create/update/fetch the TBR guard for Image Builder after turnpike.
	imageBuilderHref, err := r.CreateOrUpdateImageBuilderGuard(ctx)
	if err != nil {
		return "", err
	}
	if imageBuilderHref != "" {
		guardHrefs = append(guardHrefs, imageBuilderHref)
	}
	return r.createOrUpdateRhelCompositeGuard(ctx, guardHrefs, features)
}

func (r pulpDaoImpl) CreateOrUpdateOrgIdGuard(ctx context.Context, orgId string) (string, error) {
	return r.createOrUpdateRHIDHeaderGuard(ctx, ORG_ID_GUARD_NAME, ORG_ID_JQ_FILTER, orgId)
}

func (r pulpDaoImpl) CreateOrUpdateTurnpikeGuard(ctx context.Context) (string, error) {
	return r.createOrUpdateRHIDHeaderGuard(ctx, TURNPIKE_GUARD_NAME, TURNPIKE_JQ_FILTER, config.Get().Clients.Pulp.GuardSubjectDn)
}

// CreateOrUpdateImageBuilderGuard returns an empty href when the TBR login is unset.
// The gateway authenticates the TBR credentials and supplies the trusted X-RH-IDENTITY header.
func (r pulpDaoImpl) CreateOrUpdateImageBuilderGuard(ctx context.Context) (string, error) {
	orgID, username, err := config.Get().Clients.Pulp.ImageBuilderIdentity()
	if err != nil || orgID == "" {
		return "", err
	}

	expectedIdentity, err := json.Marshal([]string{orgID, username})
	if err != nil {
		return "", fmt.Errorf("encoding Image Builder identity: %w", err)
	}

	// Compare the pair in one guard: composite children are ORed, so separate
	// guards for the org and username would authorize either field on its own.
	jqFilter := fmt.Sprintf(`(.identity | [.org_id, .user.username]) == %s | tostring`, expectedIdentity)
	return r.createOrUpdateRHIDHeaderGuard(ctx, IMAGE_BUILDER_GUARD_NAME, jqFilter, "true")
}

func (r pulpDaoImpl) createOrUpdateRHIDHeaderGuard(ctx context.Context, name string, jqFilter string, value string) (string, error) {
	pulpHref, err := r.fetchAndUpdateHeaderGuard(ctx, name, jqFilter, value)
	if err != nil || pulpHref != "" {
		return pulpHref, err
	}
	// guard doesn't exist, so create it
	pulpHref, err = r.createRHIDHeaderGuard(ctx, name, jqFilter, value)
	if err != nil {
		guard, _ := r.FetchHeaderContentGuard(ctx, name)
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

// FetchHeaderContentGuard returns nil if no guard with this name exists.
func (r pulpDaoImpl) FetchHeaderContentGuard(ctx context.Context, name string) (*zest.HeaderContentGuardResponse, error) {
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
	guard, err := r.FetchHeaderContentGuard(ctx, name)
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

func (r pulpDaoImpl) createOrUpdateCompositeGuard(ctx context.Context, guardHrefs ...string) (string, error) {
	pulpHref, err := r.fetchOrUpdateCompositeGuard(ctx, guardHrefs...)
	if err != nil || pulpHref != "" {
		return pulpHref, err
	}
	// guard doesn't exist, so create it
	pulpHref, err = r.createCompositeGuard(ctx, guardHrefs...)
	if err != nil {
		guard, _ := r.fetchCompositeContentGuard(ctx)
		if guard == nil {
			return "", fmt.Errorf("failed to create and fetch composite content guard %w", err)
		}
		return *guard.PulpHref, nil
	}
	return pulpHref, err
}

func (r pulpDaoImpl) createCompositeGuard(ctx context.Context, guardHrefs ...string) (string, error) {
	return r.createNamedCompositeGuard(ctx, COMPOSITE_GUARD_NAME, guardHrefs)
}

func (r pulpDaoImpl) fetchCompositeContentGuard(ctx context.Context) (*zest.CompositeContentGuardResponse, error) {
	return r.FetchCompositeContentGuard(ctx, COMPOSITE_GUARD_NAME)
}

func (r pulpDaoImpl) fetchOrUpdateCompositeGuard(ctx context.Context, guardHrefs ...string) (string, error) {
	return r.fetchOrUpdateNamedCompositeGuard(ctx, COMPOSITE_GUARD_NAME, guardHrefs)
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

// FetchCompositeContentGuard returns nil if no guard with this name exists.
func (r pulpDaoImpl) FetchCompositeContentGuard(ctx context.Context, name string) (*zest.CompositeContentGuardResponse, error) {
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
	guard, err := r.FetchCompositeContentGuard(ctx, name)
	if err != nil {
		return "", err
	} else if guard == nil {
		return "", nil
	}

	if compositeChildHrefsEqual(guard.Guards, guardHrefs) {
		return *guard.PulpHref, nil
	}
	return r.UpdateCompositeGuardChildren(ctx, *guard.PulpHref, guardHrefs)
}

// UpdateCompositeGuardChildren replaces the child references on an existing composite.
func (r pulpDaoImpl) UpdateCompositeGuardChildren(ctx context.Context, href string, guardHrefs []string) (string, error) {
	ctx, client, err := getZestClient(ctx)
	if err != nil {
		return "", err
	}

	update := zest.PatchedCompositeContentGuard{
		Guards: ptrStringsFromHrefs(guardHrefs),
	}
	response, httpResp, err := client.ContentguardsCompositeAPI.ContentguardsCoreCompositePartialUpdate(ctx, href).
		PatchedCompositeContentGuard(update).Execute()
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return "", errorWithResponseBody("error updating composite guard", httpResp, err)
	}
	return *response.PulpHref, nil
}

func (r pulpDaoImpl) createOrUpdateNamedCompositeGuard(ctx context.Context, name string, guardHrefs []string) (string, error) {
	pulpHref, err := r.fetchOrUpdateNamedCompositeGuard(ctx, name, guardHrefs)
	if err != nil || pulpHref != "" {
		return pulpHref, err
	}
	pulpHref, err = r.createNamedCompositeGuard(ctx, name, guardHrefs)
	if err != nil {
		guard, _ := r.FetchCompositeContentGuard(ctx, name)
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

// ListCompositeContentGuards returns all composite guards in this domain.
func (r pulpDaoImpl) ListCompositeContentGuards(ctx context.Context) ([]zest.CompositeContentGuardResponse, error) {
	ctx, client, err := getZestClient(ctx)
	if err != nil {
		return nil, err
	}

	const pageSize int32 = 100
	var guards []zest.CompositeContentGuardResponse
	for offset := int32(0); ; {
		page, resp, err := client.ContentguardsCompositeAPI.ContentguardsCoreCompositeList(ctx, r.domainName).
			Limit(pageSize).Offset(offset).Ordering([]string{"name"}).Execute()
		if resp != nil {
			resp.Body.Close()
		}
		if err != nil {
			return nil, fmt.Errorf("listing composite content guards: %w", err)
		}

		for _, guard := range page.Results {
			// Advance by actual results, since Pulp may cap the requested limit.
			offset++
			guards = append(guards, guard)
		}

		if offset >= page.Count {
			return guards, nil
		}
		if len(page.Results) == 0 {
			return nil, fmt.Errorf("composite guard pagination returned an empty page before the end")
		}
	}
}
