package jobs

import (
	"context"
	"fmt"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/utils"
	zest "github.com/content-services/zest/release/v2026"
	"github.com/stretchr/testify/require"
)

func TestUpdateImageBuilderGuards(t *testing.T) {
	previousDomain := config.Get().Clients.Pulp.CoprDomain
	t.Cleanup(func() { config.Get().Clients.Pulp.CoprDomain = previousDomain })
	config.Get().Clients.Pulp.CoprDomain = "copr"

	ctx := context.Background()
	domains := dao.NewMockDomainDao(t)
	global := pulp_client.NewMockPulpGlobalClient(t)
	client := pulp_client.NewMockPulpClient(t)

	domains.On("List", ctx).Return([]models.Domain{{DomainName: "customer"}, {DomainName: "not-created"}, {DomainName: "cs-redhat"}}, nil)
	global.On("LookupDomain", ctx, "customer").Return("/customer/", nil)
	global.On("LookupDomain", ctx, "not-created").Return("", nil)
	global.On("LookupDomain", ctx, "cs-redhat").Return("/redhat/", nil)
	global.On("LookupDomain", ctx, "copr").Return("/copr/", nil).Once()
	client.On("ListCompositeContentGuards", ctx).Return([]zest.CompositeContentGuardResponse{}, nil).Times(3)
	factory := func(domain string) pulp_client.PulpClient {
		require.NotEqual(t, "not-created", domain)
		return client
	}
	require.NoError(t, updateImageBuilderGuards(ctx, domains, global, factory))
}

func TestUpdateImageBuilderGuardsContinuesAfterFailure(t *testing.T) {
	previousDomain := config.Get().Clients.Pulp.CoprDomain
	t.Cleanup(func() { config.Get().Clients.Pulp.CoprDomain = previousDomain })
	config.Get().Clients.Pulp.CoprDomain = ""

	ctx := context.Background()
	domains := dao.NewMockDomainDao(t)
	global := pulp_client.NewMockPulpGlobalClient(t)
	client := pulp_client.NewMockPulpClient(t)
	domains.On("List", ctx).Return([]models.Domain{{DomainName: "lookup-failure"}, {DomainName: "update-failure"}, {DomainName: "good"}}, nil)
	global.On("LookupDomain", ctx, "lookup-failure").Return("", fmt.Errorf("lookup unavailable"))
	global.On("LookupDomain", ctx, "update-failure").Return("/failed/", nil)
	global.On("LookupDomain", ctx, "good").Return("/good/", nil).Once()
	client.On("ListCompositeContentGuards", ctx).Return(nil, fmt.Errorf("update unavailable")).Once()
	client.On("ListCompositeContentGuards", ctx).Return([]zest.CompositeContentGuardResponse{}, nil).Once()
	err := updateImageBuilderGuards(ctx, domains, global, func(string) pulp_client.PulpClient { return client })
	require.ErrorContains(t, err, "lookup unavailable")
	require.ErrorContains(t, err, "update unavailable")
}

func TestAddImageBuilderToContentGuards(t *testing.T) {
	ctx := context.Background()
	client := pulp_client.NewMockPulpClient(t)
	children := []*string{utils.Ptr("/org/"), utils.Ptr("/certificate/")}
	customer := zest.CompositeContentGuardResponse{Name: "composite_guard", PulpHref: utils.Ptr("/customer/"), Guards: children}
	rhel := zest.CompositeContentGuardResponse{Name: "rhel_composite_historical", PulpHref: utils.Ptr("/rhel/"), Guards: children}
	client.On("ListCompositeContentGuards", ctx).Return([]zest.CompositeContentGuardResponse{
		customer, rhel, {Name: "unrelated-empty-composite"},
	}, nil)
	client.On("CreateOrUpdateImageBuilderGuard", ctx).Return("/image-builder/", nil)
	client.On("UpdateCompositeGuardChildren", ctx, "/customer/", []string{"/org/", "/certificate/", "/image-builder/"}).Return("/customer/", nil)
	client.On("UpdateCompositeGuardChildren", ctx, "/rhel/", []string{"/org/", "/certificate/", "/image-builder/"}).Return("/rhel/", nil)
	client.On("FetchCompositeContentGuard", ctx, customer.Name).Return(&customer, nil)
	client.On("FetchCompositeContentGuard", ctx, rhel.Name).Return(&rhel, nil)
	client.On("GetDomain").Return("test-domain")
	count, err := addImageBuilderToContentGuards(ctx, client)
	require.NoError(t, err)
	require.Equal(t, 2, count)
}

func TestAddImageBuilderRefusesEmptyComposite(t *testing.T) {
	ctx := context.Background()
	client := pulp_client.NewMockPulpClient(t)
	client.On("ListCompositeContentGuards", ctx).Return([]zest.CompositeContentGuardResponse{
		{Name: "composite_guard", PulpHref: utils.Ptr("/empty/")},
	}, nil)
	_, err := addImageBuilderToContentGuards(ctx, client)
	require.ErrorContains(t, err, "refusing to change authorization")
}

func TestAddImageBuilderReconcilesAlreadyAttachedGuard(t *testing.T) {
	ctx := context.Background()
	client := pulp_client.NewMockPulpClient(t)
	guard := zest.CompositeContentGuardResponse{
		Name: "composite_guard", PulpHref: utils.Ptr("/customer/"),
		Guards: []*string{utils.Ptr("/org/"), utils.Ptr("/image-builder/")},
	}
	client.On("ListCompositeContentGuards", ctx).Return([]zest.CompositeContentGuardResponse{guard}, nil)
	client.On("CreateOrUpdateImageBuilderGuard", ctx).Return("/image-builder/", nil).Once()
	client.On("FetchCompositeContentGuard", ctx, guard.Name).Return(&guard, nil)
	count, err := addImageBuilderToContentGuards(ctx, client)
	require.NoError(t, err)
	require.Zero(t, count)
}

func TestUpdateImageBuilderGuardsDomainListFailure(t *testing.T) {
	ctx := context.Background()
	domains := dao.NewMockDomainDao(t)
	domains.On("List", ctx).Return(nil, fmt.Errorf("database unavailable"))
	err := updateImageBuilderGuards(ctx, domains, nil, nil)
	require.ErrorContains(t, err, "database unavailable")
}
