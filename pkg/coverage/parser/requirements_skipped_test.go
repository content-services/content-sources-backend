package parser

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRequirementsSkippedEntries(t *testing.T) {
	tests := []struct {
		name, filename, content string
		skipped, packages       int
	}{
		{"requirements", "requirements.txt", "# comment\n\n--index-url https://example.com\n-r other.txt\nflask==1 \\\n --hash=sha256:abc\nflask==1\n==2\nhttps://example.com/archive.zip\n-e ./local\n", 2, 2},
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
