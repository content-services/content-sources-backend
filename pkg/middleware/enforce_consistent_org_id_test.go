package middleware

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractOrgIdsVariousFormats(t *testing.T) {
	response1 := map[string]any{
		"org_id": "test-org-123",
		"other":  "data",
	}
	orgIds1 := extractOrgIds(response1)
	assert.Equal(t, []string{"test-org-123"}, orgIds1)

	response2 := map[string]any{
		"data": []interface{}{
			map[string]interface{}{
				"uuid":   "item1",
				"org_id": "test-org-789",
			},
			map[string]interface{}{
				"uuid":   "item2",
				"org_id": "test-org-789",
			},
		},
	}
	orgIds2 := extractOrgIds(response2)
	assert.Equal(t, []string{"test-org-789", "test-org-789"}, orgIds2)

	response3 := map[string]any{
		"data": map[string]interface{}{
			"uuid":   "single-item",
			"org_id": "test-org-single",
		},
	}
	orgIds3 := extractOrgIds(response3)
	assert.Equal(t, []string{"test-org-single"}, orgIds3)

	response4 := map[string]any{
		"other": "data",
	}
	orgIds4 := extractOrgIds(response4)
	assert.Empty(t, orgIds4)

	response5 := map[string]any{
		"data": []interface{}{},
	}
	orgIds5 := extractOrgIds(response5)
	assert.Empty(t, orgIds5)
}
