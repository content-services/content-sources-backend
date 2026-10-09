package parser

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSpdxSkippedEntries(t *testing.T) {
	tests := []struct {
		name, filename, content string
		skipped, packages       int
	}{
		{"SPDX 2 JSON", "bom.spdx.json", `{"spdxVersion":"SPDX-2.3","packages":[{"name":"flask","externalRefs":[{"referenceLocator":"pkg:pypi/flask@1"}]},{"name":"missing"},{"name":"invalid","externalRefs":[{"referenceLocator":"bad"}]}]}`, 2, 1},
		{"SPDX 3 JSON", "bom.spdx.json", `{"@graph":[{"type":"software_Package","software_packageUrl":"pkg:pypi/flask@1"},{"type":"software_Package","name":"missing"},{"type":"software_File","name":"file"}]}`, 1, 1},
		{"SPDX tag-value", "bom.spdx", "SPDXVersion: SPDX-2.3\nPackageName: flask\nExternalRef: PACKAGE-MANAGER purl pkg:pypi/flask@1\nPackageName: missing\nFileName: file\n", 1, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Parse(tt.filename, strings.NewReader(tt.content))
			require.NoError(t, err)
			assert.Equal(t, tt.skipped, result.SkippedEntries)
			assert.Len(t, result.Packages, tt.packages)
		})
	}
}
