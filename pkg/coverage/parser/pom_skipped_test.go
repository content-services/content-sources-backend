package parser

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePomSkippedEntries(t *testing.T) {
	tests := []struct {
		name, filename, content string
		skipped, packages       int
	}{
		{"POM", "pom.xml", `<project xmlns="http://maven.apache.org/POM/4.0.0"><modelVersion>4.0.0</modelVersion><groupId>org.example</groupId><artifactId>app</artifactId><version>1</version><dependencies><dependency><groupId>org.example</groupId><artifactId>valid</artifactId><version>1</version></dependency><dependency><artifactId>missing-group</artifactId><version>1</version></dependency></dependencies></project>`, 1, 1},
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
