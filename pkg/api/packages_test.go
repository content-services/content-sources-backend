package api

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPackageResponseJSON(t *testing.T) {
	response := PackageResponse{
		Results: []PackageItem{
			{
				Group:    "io.smallrye.reactive",
				Name:     "smallrye-mutiny-vertx-core",
				Versions: []string{"3.16.0", "3.15.0"},
				LatestReleases: []ReleaseInfo{
					{
						Version:   "3.15.0",
						Release:   "rhlw-3001",
						CreatedAt: "2024-01-15T10:30:00Z",
					},
				},
			},
		},
		Total:  1,
		Limit:  100,
		Offset: 0,
	}

	jsonData, err := json.Marshal(response)
	assert.Nil(t, err)
	assert.Contains(t, string(jsonData), "io.smallrye.reactive")
	assert.Contains(t, string(jsonData), "latest_releases")
}
