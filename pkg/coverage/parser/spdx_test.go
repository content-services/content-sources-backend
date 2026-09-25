package parser

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse_SPDX2JSON(t *testing.T) {
	result := parseTestdata(t, "manifest.spdx.json", filepath.Join("spdx", "v2.json"))
	assert.Equal(t, FormatSPDX, result.InputFormat)
	assert.ElementsMatch(t, []Package{
		{Ecosystem: EcosystemPython, Name: "flask", Version: "3.0.3"},
		{Ecosystem: EcosystemJava, Namespace: "org.springframework", Name: "spring-core", Version: "5.3.20"},
		{Ecosystem: EcosystemJavaScript, Name: "express", Version: "4.18.2"},
	}, result.Packages)
}

func TestParse_SPDX3JSON(t *testing.T) {
	result := parseTestdata(t, "sbom.spdx.json", filepath.Join("spdx", "v3.json"))
	assert.Equal(t, FormatSPDX, result.InputFormat)
	assert.ElementsMatch(t, []Package{
		{Ecosystem: EcosystemPython, Name: "flask", Version: "3.0.3"},
		{Ecosystem: EcosystemJava, Namespace: "org.springframework", Name: "spring-web", Version: "6.1.5"},
		{Ecosystem: EcosystemJavaScript, Name: "express", Version: "4.18.2"},
	}, result.Packages)
}

func TestParse_SPDXTagValue(t *testing.T) {
	result := parseTestdata(t, "bom.spdx", filepath.Join("spdx", "tagvalue.spdx"))
	assert.Equal(t, FormatSPDX, result.InputFormat)
	assert.ElementsMatch(t, []Package{
		{Ecosystem: EcosystemPython, Name: "flask", Version: "3.0.3"},
		{Ecosystem: EcosystemJava, Namespace: "org.springframework", Name: "spring-core", Version: "5.3.20"},
		{Ecosystem: EcosystemJavaScript, Name: "express", Version: "4.18.2"},
	}, result.Packages)
}

func TestParse_SPDX2JSON_NoPURLs(t *testing.T) {
	data := `{"spdxVersion":"SPDX-2.3","packages":[` +
		`{"name":"flask","SPDXID":"SPDXRef-1","versionInfo":"3.0.3","downloadLocation":"NOASSERTION"},` +
		`{"name":"requests","SPDXID":"SPDXRef-2","versionInfo":"2.31.0","downloadLocation":"NOASSERTION"},` +
		`{"name":"bash","SPDXID":"SPDXRef-3","versionInfo":"5.1.8-9.el9","downloadLocation":"NOASSERTION",` +
		`"externalRefs":[{"referenceCategory":"SECURITY","referenceType":"cpe23Type",` +
		`"referenceLocator":"cpe:2.3:o:redhat:enterprise_linux:9:*:*:*:*:*:*:*"}]}` +
		`]}`
	result, err := Parse("sbom.spdx.json", strings.NewReader(data))
	require.NoError(t, err)
	assert.Empty(t, result.Packages)
	assert.Equal(t, 3, result.SkippedEntries)
}

func TestParse_SPDXTagValue_NoPURLs(t *testing.T) {
	data := "SPDXVersion: SPDX-2.3\nSPDXID: SPDXRef-DOCUMENT\n" +
		"PackageName: flask\nPackageVersion: 3.0.3\n" +
		"PackageName: requests\nPackageVersion: 2.31.0\n"
	result, err := Parse("bom.spdx", strings.NewReader(data))
	require.NoError(t, err)
	assert.Empty(t, result.Packages)
	assert.Equal(t, 2, result.SkippedEntries)
}

func TestParse_SPDXUnsupportedEncodings(t *testing.T) {
	tests := []struct {
		name, filename, body string
	}{
		{"yaml", "sbom.spdx.yaml", "spdxVersion: SPDX-2.3\npackages: []\n"},
		{"xml", "bom.spdx.xml", "<Document xmlns=\"http://spdx.org/rdf/terms#\"><Package/></Document>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.filename, strings.NewReader(tt.body))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "unsupported SPDX encoding")
		})
	}
}

func TestParse_SPDXJSON_SkipsLargeNonPackageArrays(t *testing.T) {
	var b strings.Builder
	b.WriteString(`{"spdxVersion":"SPDX-2.3","files":[`)
	for i := range 2000 {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(`{"fileName":"src/file-`)
		b.WriteString(strings.Repeat("x", 32))
		b.WriteString(`.py","SPDXID":"SPDXRef-File-`)
		b.WriteString(strings.Repeat("a", 8))
		b.WriteString(`"}`)
	}
	b.WriteString(`],"packages":[{"name":"flask","externalRefs":[{"referenceType":"purl","referenceLocator":"pkg:pypi/flask@3.0.3"}]}]}`)

	result, err := Parse("huge.spdx.json", strings.NewReader(b.String()))
	require.NoError(t, err)
	require.Len(t, result.Packages, 1)
	assert.Equal(t, "flask", result.Packages[0].Name)
}
