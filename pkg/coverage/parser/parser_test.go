package parser

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectFormat_CaseInsensitive(t *testing.T) {
	format, err := detectFormat("REPORT.CSV")
	assert.NoError(t, err)
	assert.Equal(t, FormatCSV, format)
}

func TestDeduplicate(t *testing.T) {
	pkgs := []Package{
		{Ecosystem: "Java", Name: "spring-core", Version: "5.3.20", Namespace: "org.springframework"},
		{Ecosystem: "Java", Name: "spring-core", Version: "5.3.20", Namespace: "org.springframework"},
		{Ecosystem: "Python", Name: "flask", Version: "3.0.3"},
		{Ecosystem: "Python", Name: "flask", Version: "3.0.3"},
		{Ecosystem: "Java", Name: "spring-core", Version: "6.0.0", Namespace: "org.springframework"},
	}
	result := deduplicate(pkgs)
	assert.Len(t, result, 3)
}

func TestParse_CSV(t *testing.T) {
	data := "vulnerability_id,packageurl\nCVE-001,pkg:pypi/flask@3.0.3\n"
	result, err := Parse("report.csv", strings.NewReader(data))
	require.NoError(t, err)
	assert.Equal(t, FormatCSV, result.InputFormat)
	assert.Len(t, result.Packages, 1)
}

func TestParse_Requirements(t *testing.T) {
	data := "flask==2.3.0\nrequests==2.31.0\n"
	result, err := Parse("requirements.txt", strings.NewReader(data))
	require.NoError(t, err)
	assert.Equal(t, FormatRequirements, result.InputFormat)
	assert.Len(t, result.Packages, 2)
}

func TestParse_UnsupportedFormat(t *testing.T) {
	_, err := Parse("data.zip", strings.NewReader("binary data"))
	assert.Error(t, err)
}
