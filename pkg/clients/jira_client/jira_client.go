package jira_client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	jirav3 "github.com/ctreminiom/go-atlassian/v2/jira/v3"
	model "github.com/ctreminiom/go-atlassian/v2/pkg/infra/models"
)

const jiraPageSize = 100

type JiraField struct {
	ID   string
	Name string
}

type JiraIssue struct {
	ID     string                     `json:"id"`
	Key    string                     `json:"key"`
	Fields map[string]json.RawMessage `json:"fields"`
}

type JiraPage struct {
	Issues        []JiraIssue `json:"issues"`
	NextPageToken string      `json:"nextPageToken"`
}

type JiraClient interface {
	Fields(ctx context.Context) ([]JiraField, error)
	Search(ctx context.Context, jql string, fields []string, nextPageToken string) (JiraPage, error)
	Issue(ctx context.Context, key string, fields []string) (JiraIssue, error)
}

type AtlassianJiraClient struct {
	client *jirav3.Client
}

func NewAtlassianJiraClient(url, user, token string) (*AtlassianJiraClient, error) {
	if url == "" || user == "" || token == "" {
		return nil, errors.New("clients.jira url, user, and token must be set")
	}

	client, err := jirav3.New(nil, url)
	if err != nil {
		return nil, fmt.Errorf("create Jira client: %w", err)
	}
	client.Auth.SetBasicAuth(user, token)
	client.Auth.SetUserAgent("content-sources-backend")

	return &AtlassianJiraClient{client: client}, nil
}

func (c *AtlassianJiraClient) Fields(ctx context.Context) ([]JiraField, error) {
	fields, _, err := c.client.Issue.Field.Gets(ctx)
	if err != nil {
		return nil, fmt.Errorf("list Jira fields: %w", err)
	}

	result := make([]JiraField, 0, len(fields))
	for _, field := range fields {
		if field != nil {
			result = append(result, JiraField{ID: field.ID, Name: field.Name})
		}
	}
	return result, nil
}

func (c *AtlassianJiraClient) Search(ctx context.Context, jql string, fields []string, nextPageToken string) (JiraPage, error) {
	_, response, callErr := c.client.Issue.Search.SearchJQL(ctx, jql, fields, nil, jiraPageSize, nextPageToken)
	var page JiraPage
	if err := decodeJiraResponse(response, callErr, &page); err != nil {
		return JiraPage{}, fmt.Errorf("search Jira issues: %w", err)
	}
	return page, nil
}

func (c *AtlassianJiraClient) Issue(ctx context.Context, key string, fields []string) (JiraIssue, error) {
	_, response, callErr := c.client.Issue.Get(ctx, key, fields, nil)
	var issue JiraIssue
	if err := decodeJiraResponse(response, callErr, &issue); err != nil {
		return JiraIssue{}, fmt.Errorf("get Jira issue %s: %w", key, err)
	}
	return issue, nil
}

// go-atlassian's typed Jira issue drops custom fields. Decode its retained raw
// response while still using the client for requests and authentication.
func decodeJiraResponse(response *model.ResponseScheme, callErr error, target any) error {
	if response == nil || response.Code < http.StatusOK || response.Code >= http.StatusMultipleChoices {
		if callErr != nil {
			return callErr
		}
		return errors.New("jira returned no successful response")
	}
	if err := json.Unmarshal(response.Bytes.Bytes(), target); err != nil {
		return fmt.Errorf("decode Jira response: %w", err)
	}
	return nil
}
