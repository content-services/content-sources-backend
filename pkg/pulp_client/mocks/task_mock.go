package pulp_client

import zest "github.com/content-services/zest/release/v3"

func (r *MockPulpClient) GetTask(taskHref string) (zest.TaskResponse, error) {
	args := r.Called(taskHref)
	if v, ok := args.Get(0).(zest.TaskResponse); ok {
		return v, nil
	}
	response := zest.TaskResponse{}
	response.SetState("completed")
	return response, nil
}
