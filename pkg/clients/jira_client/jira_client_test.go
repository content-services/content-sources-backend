package jira_client

import (
	"errors"
	"testing"

	model "github.com/ctreminiom/go-atlassian/v2/pkg/infra/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeJiraResponseKeepsCustomFields(t *testing.T) {
	response := &model.ResponseScheme{Code: 200}
	response.Bytes.WriteString(`{"key":"LTWL-1","fields":{"customfield_10632":"pkg:test/example"}}`)

	var issue JiraIssue
	err := decodeJiraResponse(response, errors.New("typed model could not decode"), &issue)
	require.NoError(t, err)
	assert.Equal(t, "LTWL-1", issue.Key)
	assert.JSONEq(t, `"pkg:test/example"`, string(issue.Fields["customfield_10632"]))
}

func TestNewAtlassianJiraClientRequiresConfiguration(t *testing.T) {
	_, err := NewAtlassianJiraClient("", "", "")
	assert.ErrorContains(t, err, "clients.jira")
}
