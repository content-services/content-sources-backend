package pulp_client

import zest "github.com/content-services/zest/release/v3"

// Creates a remote
func (r pulpDaoImpl) GetTask(taskHref string) (zest.TaskResponse, error) {
	task, _, err := r.client.TasksApi.TasksRead(r.ctx, taskHref).Execute()

	if err != nil {
		return zest.TaskResponse{}, err
	}

	return *task, nil
}
