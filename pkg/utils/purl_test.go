package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePURL(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		typeName  string
		namespace string
		nameValue string
		version   string
		fullName  string
		language  string
	}{
		{
			name:      "Maven",
			input:     "pkg:maven/org.springframework/spring-core@5.3.20?type=jar#sources",
			typeName:  "maven",
			namespace: "org.springframework",
			nameValue: "spring-core",
			version:   "5.3.20",
			fullName:  "org.springframework:spring-core",
			language:  "java",
		},
		{
			name:      "PyPI",
			input:     "pkg:PYPI/flask@2.3.0",
			typeName:  "pypi",
			nameValue: "flask",
			version:   "2.3.0",
			fullName:  "flask",
			language:  "python",
		},
		{
			name:      "scoped npm package",
			input:     "pkg:npm/%40angular/core@15.0.0",
			typeName:  "npm",
			namespace: "@angular",
			nameValue: "core",
			version:   "15.0.0",
			fullName:  "@angular/core",
			language:  "javascript",
		},
		{
			name:      "Cargo",
			input:     "pkg:cargo/serde@1.0.0",
			typeName:  "cargo",
			nameValue: "serde",
			version:   "1.0.0",
			fullName:  "serde",
			language:  "rust",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			parsed := ParsePURL(test.input)
			require.NotNil(t, parsed)
			assert.Equal(t, test.typeName, parsed.Type)
			assert.Equal(t, test.namespace, parsed.Namespace)
			assert.Equal(t, test.nameValue, parsed.Name)
			assert.Equal(t, test.version, parsed.Version)
			assert.Equal(t, test.fullName, parsed.FullName())

			language := parsed.Language()
			require.NotNil(t, language)
			assert.Equal(t, test.language, *language)
		})
	}
}

func TestPURLLanguageReturnsNilForUnknownType(t *testing.T) {
	parsed := ParsePURL("pkg:generic/example@1.0.0")
	require.NotNil(t, parsed)
	assert.Nil(t, parsed.Language())
}

func TestParsePURLReturnsNilForInvalidPURLs(t *testing.T) {
	for _, input := range []string{"", "not-a-purl", "pkg:", "pkg:maven/"} {
		assert.Nil(t, ParsePURL(input), input)
	}
}
