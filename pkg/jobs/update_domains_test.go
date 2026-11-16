package jobs

import (
	"context"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/stretchr/testify/assert"
)

func TestUpdateDomains_Success(t *testing.T) {
	ctx := context.Background()

	mockDomainDao := dao.NewMockDomainDao(t)
	mockPulpClient := pulp_client.NewMockPulpGlobalClient(t)

	domains := []models.Domain{
		{OrgId: "org-1", DomainName: "domain-1"},
		{OrgId: "org-2", DomainName: "domain-2"},
	}

	mockDomainDao.On("List", ctx).Return(domains, nil)

	mockPulpClient.On("UpdateDomainIfNeeded", ctx, "domain-1").Return(nil).Once()
	mockPulpClient.On("UpdateDomainIfNeeded", ctx, "domain-2").Return(nil).Once()

	updateDomainsRun(ctx, mockDomainDao, mockPulpClient)
	mockDomainDao.AssertExpectations(t)
	mockPulpClient.AssertExpectations(t)
}

func TestUpdateDomains_PartialFailure(t *testing.T) {
	ctx := context.Background()

	mockDomainDao := dao.NewMockDomainDao(t)
	mockPulpClient := pulp_client.NewMockPulpGlobalClient(t)

	domains := []models.Domain{
		{OrgId: "org-1", DomainName: "domain-1"},
		{OrgId: "org-2", DomainName: "domain-2"},
		{OrgId: "org-3", DomainName: "domain-3"},
	}

	mockDomainDao.On("List", ctx).Return(domains, nil)

	mockPulpClient.On("UpdateDomainIfNeeded", ctx, "domain-1").Return(assert.AnError).Once()
	mockPulpClient.On("UpdateDomainIfNeeded", ctx, "domain-2").Return(nil).Once()
	mockPulpClient.On("UpdateDomainIfNeeded", ctx, "domain-3").Return(nil).Once()

	updateDomainsRun(ctx, mockDomainDao, mockPulpClient)

	mockDomainDao.AssertExpectations(t)
	mockPulpClient.AssertExpectations(t)
}

func TestUpdateDomains_NoDomains(t *testing.T) {
	ctx := context.Background()

	mockDomainDao := dao.NewMockDomainDao(t)
	mockPulpClient := pulp_client.NewMockPulpGlobalClient(t)

	mockDomainDao.On("List", ctx).Return([]models.Domain{}, nil)

	updateDomainsRun(ctx, mockDomainDao, mockPulpClient)

	mockDomainDao.AssertExpectations(t)
	mockPulpClient.AssertNotCalled(t, "UpdateDomainIfNeeded")
}
