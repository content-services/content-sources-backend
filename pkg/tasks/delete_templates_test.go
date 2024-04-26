package tasks

import (
	"context"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type DeleteTemplatesSuite struct {
	suite.Suite
	mockDaoRegistry *dao.MockDaoRegistry
	MockQueue       queue.MockQueue
	Queue           queue.Queue
}

func TestDeleteTemplatesSuite(t *testing.T) {
	suite.Run(t, new(DeleteTemplatesSuite))
}

func (s *DeleteTemplatesSuite) SetupTest() {
	s.mockDaoRegistry = dao.GetMockDaoRegistry(s.T())
	s.MockQueue = *queue.NewMockQueue(s.T())
	s.Queue = &s.MockQueue
}

func (s *DeleteTemplatesSuite) TestDeleteTemplates() {
	ctx := context.Background()
	template := api.TemplateResponse{OrgID: "OrgId", UUID: uuid.NewString()}
	task := models.TaskInfo{
		Id:             uuid.UUID{},
		OrgId:          template.OrgID,
		RepositoryUUID: uuid.Nil,
	}

	s.mockDaoRegistry.Template.On("Fetch", ctx, template.UUID).Return([]models.Template{}, nil).Once()
	s.mockDaoRegistry.Template.On("Delete", ctx, template.OrgID, template.UUID).Return(nil).Once()

	payload := DeleteTemplatesPayload{
		TemplateUUID: template.UUID,
	}
	deleteTemplatesTask := DeleteTemplates{
		daoReg:  s.mockDaoRegistry.ToDaoRegistry(),
		payload: &payload,
		task:    &task,
		ctx:     ctx,
	}
	templateErr := deleteTemplatesTask.Run()
	assert.NoError(s.T(), templateErr)
}
