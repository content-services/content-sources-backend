package helpers

import (
	"context"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	zest "github.com/content-services/zest/release/v2025"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type PulpDistributionHelperTest struct {
	suite.Suite
}

func TestPulpDistributionHelperSuite(t *testing.T) {
	suite.Run(t, new(PulpDistributionHelperTest))
}

func (s *PulpDistributionHelperTest) SetupSuite() {
	config.Get().Clients.Pulp.RepoContentGuards = true
}

func (s *PulpDistributionHelperTest) TestCustomDistributionCreate() {
	ctx := context.Background()
	mockPulp := pulp_client.NewMockPulpClient(s.T())
	helper := NewPulpDistributionHelper(ctx, mockPulp)

	pubHref := "pubhref"
	distPath := "dispatch"
	distName := "distName"
	orgId := "custom"
	guardHref := "guardhref"
	taskHref := "taskHref"
	taskResp := zest.TaskResponse{
		PulpHref: &taskHref,
	}

	mockPulp.On("CreateOrUpdateGuardsForOrg", ctx, orgId).Return(guardHref, nil)
	mockPulp.On("CreateRpmDistribution", ctx, pubHref, distName, distPath, &guardHref).Return(&taskHref, nil)
	mockPulp.On("PollTask", ctx, taskHref).Return(&taskResp, nil)
	created, err := helper.CreateDistribution(api.RepositoryResponse{OrgID: orgId}, pubHref, distName, distPath)
	assert.Nil(s.T(), err)
	assert.Equal(s.T(), created, &taskResp)
}

func (s *PulpDistributionHelperTest) TestRedHatDistributionCreate() {
	ctx := context.Background()
	mockPulp := pulp_client.NewMockPulpClient(s.T())
	helper := NewPulpDistributionHelper(ctx, mockPulp)

	pubHref := "pubhref"
	distPath := "dispatch"
	distName := "distName"
	orgId := config.RedHatOrg
	taskHref := "taskHref"
	var guardHref *string
	taskResp := zest.TaskResponse{
		PulpHref: &taskHref,
	}

	mockPulp.On("CreateRpmDistribution", ctx, pubHref, distName, distPath, guardHref).Return(&taskHref, nil)
	mockPulp.On("PollTask", ctx, taskHref).Return(&taskResp, nil)
	created, err := helper.CreateDistribution(api.RepositoryResponse{OrgID: orgId}, pubHref, distName, distPath)
	assert.Nil(s.T(), err)
	assert.Equal(s.T(), created, &taskResp)
}

func (s *PulpDistributionHelperTest) Test_UpdateRPMDistribution_WhenPulpUpdateSends200() {
	ctx := context.Background()
	mockPulp := pulp_client.NewMockPulpClient(s.T())
	helper := NewPulpDistributionHelper(ctx, mockPulp)

	taskHref := ""
	var nilTaskHref *string
	pubHref := "pubHref"
	distHref := "distHref"
	distPath := "dispatch"
	distName := "distName"
	var guardHref *string
	orgId := config.RedHatOrg
	taskResp := zest.RpmRpmDistributionResponse{
		PulpHref: &distHref,
	}

	mockPulp.On("FindDistributionByPath", ctx, distPath).Return(&taskResp, nil)
	mockPulp.On("UpdateRpmDistribution", ctx, distHref, pubHref, distName, distPath, guardHref).Return(taskHref, nil)

	returnedDistHref, returnedTaskHref, err := helper.CreateOrUpdateDistribution(api.RepositoryResponse{OrgID: orgId}, pubHref, distName, distPath)

	// fmt.Println("Top, returnedDistHref: ", returnedDistHref)
	// fmt.Println("Top, *taskResp.PulpHref: ", *taskResp.PulpHref)
	// fmt.Println("Top, returnedTaskHref: ", returnedTaskHref)
	// fmt.Println("Top, nilTask ", nilTask)

	assert.Equal(s.T(), *taskResp.PulpHref, returnedDistHref)
	assert.Equal(s.T(), nilTaskHref, returnedTaskHref)
	assert.NoError(s.T(), err)
}

func (s *PulpDistributionHelperTest) TestRedHatDistributionUpdate() {
	ctx := context.Background()
	mockPulp := pulp_client.NewMockPulpClient(s.T())
	helper := NewPulpDistributionHelper(ctx, mockPulp)

	pubHref := "pubhref"
	distPath := "dispatch"
	distName := "distName"
	distHref := "distHref"
	orgId := config.RedHatOrg
	taskHref := "taskHref"
	var guardHref *string
	taskResp := zest.TaskResponse{
		PulpHref: &taskHref,
	}

	mockPulp.On("UpdateRpmDistribution", ctx, distHref, pubHref, distName, distPath, guardHref).Return(taskHref, nil)
	mockPulp.On("PollTask", ctx, taskHref).Return(&taskResp, nil)
	mockPulp.On("FindDistributionByPath", ctx, distPath).Return(&zest.RpmRpmDistributionResponse{
		PulpHref: &distHref,
	}, nil)

	_, _, err := helper.CreateOrUpdateDistribution(api.RepositoryResponse{OrgID: orgId}, pubHref, distName, distPath)
	assert.NoError(s.T(), err)
}

func (s *PulpDistributionHelperTest) TestRedHatDistributionWithFeatureCreate() {
	ctx := context.Background()
	mockPulp := pulp_client.NewMockPulpClient(s.T())
	helper := NewPulpDistributionHelper(ctx, mockPulp)

	feature := "abacadaba"
	pubHref := "pubhref"
	distPath := "dispatch"
	distName := "distName"
	orgId := config.RedHatOrg
	taskHref := "taskHref"
	guardHref := "besthref"
	taskResp := zest.TaskResponse{
		PulpHref: &taskHref,
	}

	mockPulp.On("CreateOrUpdateGuardsForRhelRepo", ctx, feature).Return(guardHref, nil)
	mockPulp.On("CreateRpmDistribution", ctx, pubHref, distName, distPath, &guardHref).Return(&taskHref, nil)
	mockPulp.On("PollTask", ctx, taskHref).Return(&taskResp, nil)
	created, err := helper.CreateDistribution(api.RepositoryResponse{OrgID: orgId, FeatureName: feature}, pubHref, distName, distPath)
	assert.Nil(s.T(), err)
	assert.Equal(s.T(), created, &taskResp)
}
