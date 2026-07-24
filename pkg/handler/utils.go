package handler

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/rbac"
	"github.com/content-services/content-sources-backend/pkg/tasks"
	"github.com/content-services/content-sources-backend/pkg/tasks/client"
	"github.com/content-services/content-sources-backend/pkg/tasks/payloads"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	"github.com/content-services/content-sources-backend/pkg/utils"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

func GetHeader(c echo.Context, key string, defvalues []string) []string {
	val, ok := c.Request().Header[key]
	if !ok {
		return defvalues
	}
	return val
}

func removeEndSuffix(source string, suffix string) string {
	output := source
	j := len(source) - 1

	for j > 0 && strings.HasSuffix(output, suffix) {
		output = strings.TrimSuffix(output, suffix)
	}

	return output
}

func addRepoRoute(e *echo.Group, method string, path string, h echo.HandlerFunc, verb rbac.Verb, m ...echo.MiddlewareFunc) {
	e.Add(method, path, h, m...)
	rbac.ServicePermissions.Add(method, path, rbac.ResourceRepositories, verb)
}

func addTemplateRoute(e *echo.Group, method string, path string, h echo.HandlerFunc, verb rbac.Verb, m ...echo.MiddlewareFunc) {
	e.Add(method, path, h, m...)
	rbac.ServicePermissions.Add(method, path, rbac.ResourceTemplates, verb)
}

func preprocessInput(input *api.ContentUnitSearchRequest) {
	if input == nil {
		return
	}
	for i, url := range input.URLs {
		input.URLs[i] = removeEndSuffix(url, "/")
	}
	if input.Limit == nil {
		input.Limit = utils.Ptr(api.ContentUnitSearchRequestLimitDefault)
	}
	if *input.Limit > api.ContentUnitSearchRequestLimitMaximum {
		*input.Limit = api.ContentUnitSearchRequestLimitMaximum
	}
}

func extractUploadUuid(href string) string {
	uuid := strings.TrimSuffix(href, "/")
	lastIndex := strings.LastIndex(uuid, "/")
	if lastIndex == -1 {
		return ""
	}
	return uuid[lastIndex+1:]
}

func fetchSnapshotUUIDsForRepos(ctx context.Context, dao *dao.DaoRegistry, orgID string, date time.Time, URLs, UUIDs []string) ([]string, error) {
	var snapshotUUIDs []string

	repoUUIDs, err := dao.RepositoryConfig.FetchRepoUUIDsByURLs(ctx, orgID, URLs)
	if err != nil {
		return []string{}, ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error fetching repos by provided URLs", err.Error())
	}

	snapshotsResp, err := dao.Snapshot.FetchSnapshotsByDateAndRepository(ctx, orgID, api.ListSnapshotByDateRequest{
		RepositoryUUIDS: utils.Deduplicate(append(UUIDs, repoUUIDs...)),
		Date:            date,
	})
	if err != nil {
		return []string{}, ce.NewErrorResponse(ce.HttpCodeForDaoError(err), "Error fetching snapshots", err.Error())
	}

	for _, s := range snapshotsResp.Data {
		if s.Match != nil {
			snapshotUUIDs = append(snapshotUUIDs, s.Match.UUID)
		} else {
			return []string{}, ce.NewErrorResponse(http.StatusBadRequest, "Error fetching snapshots", "One or more provided repositories don't have a snapshot.")
		}
	}

	return snapshotUUIDs, nil
}

func enqueueTask(tc client.TaskClient, task queue.Task) (uuid.UUID, error) {
	taskID, err := tc.Enqueue(task)
	if err != nil {
		logger := tasks.LogForTask(taskID.String(), task.Typename, task.RequestID)
		logger.Error().Msg("error enqueuing task")
		return uuid.Nil, err
	}

	return taskID, nil
}

func enqueueUpdateSnapshotPublishedTask(c echo.Context, tc client.TaskClient, repoUUID, snapshotUUID string, published bool, dependencies ...uuid.UUID) (uuid.UUID, error) {
	accountID, orgID := getAccountIdOrgId(c)

	task := queue.Task{
		Typename:     config.UpdateSnapshotPublishedTask,
		Payload:      tasks.UpdateSnapshotPublishedPayload{SnapshotUUID: snapshotUUID, Published: published},
		OrgId:        orgID,
		AccountId:    accountID,
		ObjectUUID:   utils.Ptr(repoUUID),
		ObjectType:   utils.Ptr(string(config.ObjectTypeRepository)),
		RequestID:    c.Response().Header().Get(config.HeaderRequestId),
		Dependencies: dependencies,
	}

	return enqueueTask(tc, task)
}

func enqueueUpdateLatestSnapshotTask(c echo.Context, tc client.TaskClient, repoUUID string, dependencies ...uuid.UUID) (uuid.UUID, error) {
	accountID, orgID := getAccountIdOrgId(c)

	task := queue.Task{
		Typename:     config.UpdateLatestSnapshotTask,
		Payload:      tasks.UpdateLatestSnapshotPayload{RepositoryConfigUUID: repoUUID},
		OrgId:        orgID,
		AccountId:    accountID,
		ObjectUUID:   utils.Ptr(repoUUID),
		ObjectType:   utils.Ptr(config.ObjectTypeRepository),
		RequestID:    c.Response().Header().Get(config.HeaderRequestId),
		Dependencies: dependencies,
	}

	return enqueueTask(tc, task)
}

func enqueueUpdateTemplateContentTask(c echo.Context, tc client.TaskClient, repoUUID, templateUUID, templateOrg string, dependencies ...uuid.UUID) (uuid.UUID, error) {
	accountID, _ := getAccountIdOrgId(c)

	task := queue.Task{
		Typename:     config.UpdateTemplateContentTask,
		Payload:      payloads.UpdateTemplateContentPayload{TemplateUUID: templateUUID, RepoConfigUUIDs: []string{repoUUID}},
		OrgId:        templateOrg,
		AccountId:    accountID,
		ObjectUUID:   utils.Ptr(templateUUID),
		ObjectType:   utils.Ptr(string(config.ObjectTypeTemplate)),
		RequestID:    c.Response().Header().Get(config.HeaderRequestId),
		Dependencies: dependencies,
	}

	return enqueueTask(tc, task)
}
