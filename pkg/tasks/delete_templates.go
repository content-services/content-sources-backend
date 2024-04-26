package tasks

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
)

type DeleteTemplatesPayload struct {
	TemplateUUID string
}

type DeleteTemplates struct {
	daoReg  *dao.DaoRegistry
	payload *DeleteTemplatesPayload
	task    *models.TaskInfo
	ctx     context.Context
}

func DeleteTemplateHandler(ctx context.Context, task *models.TaskInfo, _ *queue.Queue) error {
	opts := DeleteTemplatesPayload{}
	if err := json.Unmarshal(task.Payload, &opts); err != nil {
		return fmt.Errorf("payload incorrect type for " + config.DeleteTemplatesTask)
	}
	daoReg := dao.GetDaoRegistry(db.DB)

	dt := DeleteTemplates{
		daoReg:  daoReg,
		payload: &opts,
		task:    task,
		ctx:     ctx,
	}
	return dt.Run()
}

func (d *DeleteTemplates) Run() error {
	err := d.deleteTemplate()

	if err != nil {
		return err
	}
	return nil
}

func (d *DeleteTemplates) deleteTemplate() error {
	err := d.daoReg.Template.Delete(d.ctx, d.task.OrgId, d.payload.TemplateUUID)
	if err != nil {
		return err
	}
	return nil
}
