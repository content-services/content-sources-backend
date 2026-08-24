package parser

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func parseTestdata(t *testing.T, parseName, testdataPath string) *ParseResult {
	t.Helper()
	f, err := os.Open(filepath.Join("fixtures", testdataPath))
	require.NoError(t, err)
	t.Cleanup(func() { _ = f.Close() })

	result, err := Parse(parseName, f)
	require.NoError(t, err)
	return result
}

func TestSniffSBOMFormat(t *testing.T) {
	tests := []struct {
		path   string
		want   string
		wantOK bool
	}{
		{filepath.Join("fixtures", "cyclonedx", "bom.json"), FormatCycloneDX, true},
		{filepath.Join("fixtures", "cyclonedx", "tools.cdx.xml"), FormatCycloneDX, true},
		{filepath.Join("fixtures", "spdx", "v2.json"), FormatSPDX, true},
		{filepath.Join("fixtures", "spdx", "v3.json"), FormatSPDX, true},
		{filepath.Join("fixtures", "spdx", "tagvalue.spdx"), FormatSPDX, true},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			f, err := os.Open(tt.path)
			require.NoError(t, err)
			defer f.Close()

			got, err := sniffSBOMFormat(bufio.NewReader(f))
			require.NoError(t, err)
			if tt.wantOK {
				assert.Equal(t, tt.want, got)
			} else {
				assert.Empty(t, got)
			}
		})
	}
}

func TestPeekSBOMStart(t *testing.T) {
	start, err := peekSBOMStart(bufio.NewReader(strings.NewReader("  \n{")))
	require.NoError(t, err)
	assert.Equal(t, byte('{'), start)
}

func TestDiscardUTF8BOM(t *testing.T) {
	r := bufio.NewReader(bytes.NewReader(append([]byte{0xEF, 0xBB, 0xBF}, '{')))
	discardUTF8BOM(r)
	b, err := r.ReadByte()
	require.NoError(t, err)
	assert.Equal(t, byte('{'), b)
}

func TestReadJSONObject_SkipsUnknownFields(t *testing.T) {
	raw := `{"files":[{"fileName":"a.py"},{"fileName":"b.py"}],"name":"flask"}`
	dec := json.NewDecoder(strings.NewReader(raw))
	var name string
	err := readJSONObject(dec, map[string]jsonFieldHandler{
		"name": func(d *json.Decoder) error { return decodeJSONString(d, &name) },
	})
	require.NoError(t, err)
	assert.Equal(t, "flask", name)
}

func TestHasAnySuffix(t *testing.T) {
	assert.True(t, hasAnySuffix("app.cdx.json", ".cdx.json", ".cdx.xml"))
	assert.False(t, hasAnySuffix("app.cdx.yml", ".cdx.json", ".cdx.xml"))
}
