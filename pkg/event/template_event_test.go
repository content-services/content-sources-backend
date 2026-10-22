package event

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTemplateEventAdvisoryIDsJSON(t *testing.T) {
	base := MapTemplateResponse(api.TemplateResponse{
		UUID:            "3642fd52-585a-4c66-ba92-29d7f867e4b6",
		Name:            "my-template",
		OrgID:           "12345",
		Arch:            "x86_64",
		Version:         "9",
		Date:            time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC),
		RepositoryUUIDS: []string{},
	})

	t.Run("omitted when unset or empty", func(t *testing.T) {
		raw, err := json.Marshal(base)
		require.NoError(t, err)
		assert.NotContains(t, string(raw), "advisory_ids")
	})

	t.Run("errata ids when lookup succeeded", func(t *testing.T) {
		base.AdvisoryIDs = []string{"RHBA-2024:1", "RHSA-2024:2"}
		raw, err := json.Marshal(base)
		require.NoError(t, err)
		assert.NotContains(t, string(raw), `"advisory_ids":null`)
		assertExistingTemplateFields(t, raw)
		var decoded TemplateEvent
		require.NoError(t, json.Unmarshal(raw, &decoded))
		assert.Equal(t, []string{"RHBA-2024:1", "RHSA-2024:2"}, decoded.AdvisoryIDs)
	})
}

func assertExistingTemplateFields(t *testing.T, raw []byte) {
	t.Helper()
	body := string(raw)
	for _, field := range []string{`"uuid"`, `"name"`, `"org_id"`, `"description"`, `"arch"`, `"version"`, `"date"`, `"repository_uuids"`, `"rhsm_environment_id"`} {
		assert.Contains(t, body, field)
	}
}
