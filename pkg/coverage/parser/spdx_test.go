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

// TestParse_SPDX2JSON_ClairStyle_NoPURLs covers Clair/Scanner V4 style SPDX 2.3 output, which
// identifies packages via name/versionInfo/packageFileName instead of a Package URL.
func TestParse_SPDX2JSON_ClairStyle_NoPURLs(t *testing.T) {
	data := `{"spdxVersion":"SPDX-2.3","packages":[` +
		`{"name":"org.springframework:spring-core","SPDXID":"SPDXRef-1","versionInfo":"5.3.20",` +
		`"packageFileName":"maven:usr/share/app/spring-core-5.3.20.jar","downloadLocation":"NOASSERTION"},` +
		`{"name":"flask","SPDXID":"SPDXRef-2","versionInfo":"3.0.3",` +
		`"packageFileName":"python:usr/lib/python3.11/site-packages/flask","downloadLocation":"NOASSERTION"},` +
		`{"name":"bash","SPDXID":"SPDXRef-3","versionInfo":"5.1.8-9.el9",` +
		`"packageFileName":"sqlite:var/lib/rpm/rpmdb.sqlite","downloadLocation":"NOASSERTION"},` +
		`{"name":"github.com/foo/bar","SPDXID":"SPDXRef-4","versionInfo":"v1.2.3",` +
		`"packageFileName":"go:usr/bin/app","downloadLocation":"NOASSERTION"}` +
		`]}`
	result, err := Parse("sbom.spdx.json", strings.NewReader(data))
	require.NoError(t, err)
	assert.ElementsMatch(t, []Package{
		{Ecosystem: EcosystemJava, Namespace: "org.springframework", Name: "spring-core", Version: "5.3.20"},
		{Ecosystem: EcosystemPython, Name: "flask", Version: "3.0.3"},
	}, result.Packages)
	assert.Equal(t, 2, result.SkippedEntries)
}

// TestParse_SPDX2JSON_PURLTakesPrecedence ensures a valid PURL is preferred over field inference.
func TestParse_SPDX2JSON_PURLTakesPrecedence(t *testing.T) {
	data := `{"spdxVersion":"SPDX-2.3","packages":[` +
		`{"name":"flask","SPDXID":"SPDXRef-1","versionInfo":"3.0.3",` +
		`"packageFileName":"python:usr/lib/python3.11/site-packages/flask",` +
		`"externalRefs":[{"referenceType":"purl","referenceLocator":"pkg:pypi/flask@3.0.4"}]}` +
		`]}`
	result, err := Parse("sbom.spdx.json", strings.NewReader(data))
	require.NoError(t, err)
	assert.Equal(t, []Package{
		{Ecosystem: EcosystemPython, Name: "flask", Version: "3.0.4"},
	}, result.Packages)
	assert.Equal(t, 0, result.SkippedEntries)
}

// TestParse_SPDXTagValue_ClairStyle_NoPURLs mirrors the JSON Clair-style test for tag-value SPDX.
func TestParse_SPDXTagValue_ClairStyle_NoPURLs(t *testing.T) {
	data := "SPDXVersion: SPDX-2.3\nSPDXID: SPDXRef-DOCUMENT\n" +
		"PackageName: org.springframework:spring-core\nPackageVersion: 5.3.20\n" +
		"PackageFileName: maven:usr/share/app/spring-core-5.3.20.jar\n" +
		"PackageName: flask\nPackageVersion: 3.0.3\n" +
		"PackageFileName: python:usr/lib/python3.11/site-packages/flask\n" +
		"PackageName: bash\nPackageVersion: 5.1.8-9.el9\n" +
		"PackageFileName: sqlite:var/lib/rpm/rpmdb.sqlite\n"
	result, err := Parse("bom.spdx", strings.NewReader(data))
	require.NoError(t, err)
	assert.ElementsMatch(t, []Package{
		{Ecosystem: EcosystemJava, Namespace: "org.springframework", Name: "spring-core", Version: "5.3.20"},
		{Ecosystem: EcosystemPython, Name: "flask", Version: "3.0.3"},
	}, result.Packages)
	assert.Equal(t, 1, result.SkippedEntries)
}

func TestInferFromSPDX2Fields(t *testing.T) {
	tests := []struct {
		name            string
		pkgName         string
		version         string
		packageFileName string
		want            *Package
	}{
		{
			name:            "maven",
			pkgName:         "org.springframework:spring-core",
			version:         "5.3.20",
			packageFileName: "maven:usr/share/app/spring-core-5.3.20.jar",
			want:            &Package{Ecosystem: EcosystemJava, Namespace: "org.springframework", Name: "spring-core", Version: "5.3.20"},
		},
		{
			name:            "python",
			pkgName:         "flask",
			version:         "3.0.3",
			packageFileName: "python:usr/lib/python3.11/site-packages/flask",
			want:            &Package{Ecosystem: EcosystemPython, Name: "flask", Version: "3.0.3"},
		},
		{
			name:            "maven without group is skipped",
			pkgName:         "spring-core",
			version:         "5.3.20",
			packageFileName: "maven:usr/share/app/spring-core-5.3.20.jar",
			want:            nil,
		},
		{
			name:            "rpm is skipped",
			pkgName:         "bash",
			version:         "5.1.8-9.el9",
			packageFileName: "sqlite:var/lib/rpm/rpmdb.sqlite",
			want:            nil,
		},
		{
			name:            "go is skipped",
			pkgName:         "github.com/foo/bar",
			version:         "v1.2.3",
			packageFileName: "go:usr/bin/app",
			want:            nil,
		},
		{
			name:            "missing version is skipped",
			pkgName:         "flask",
			version:         "",
			packageFileName: "python:usr/lib/python3.11/site-packages/flask",
			want:            nil,
		},
		{
			name:            "missing packageFileName is skipped",
			pkgName:         "flask",
			version:         "3.0.3",
			packageFileName: "",
			want:            nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := inferFromSPDX2Fields(tt.pkgName, tt.version, tt.packageFileName)
			assert.Equal(t, tt.want, got)
		})
	}
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
