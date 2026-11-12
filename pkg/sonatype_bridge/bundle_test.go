package sonatype_bridge

import (
	"archive/zip"
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildBundleZip_FlatAndOrdered(t *testing.T) {
	files := []bundleFile{
		{Name: "lib-1.0.jar", Data: []byte("jar")},
		{Name: "lib-1.0.pom", Data: []byte("<project/>")},
		{Name: "lib-1.0-sources.jar", Data: []byte("src")},
	}
	data, err := buildBundleZip(files)
	require.NoError(t, err)

	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	require.Len(t, zr.File, 3)
	for i, f := range zr.File {
		assert.Equal(t, files[i].Name, f.Name)
		assert.False(t, f.FileInfo().IsDir())
		assert.True(t, f.Modified.Equal(bundleModTime), f.Name)
	}
	assert.Equal(t, map[string][]byte{
		"lib-1.0.jar":         []byte("jar"),
		"lib-1.0.pom":         []byte("<project/>"),
		"lib-1.0-sources.jar": []byte("src"),
	}, readZip(t, data))
}

func TestBuildBundleZip_Deterministic(t *testing.T) {
	files := []bundleFile{
		{Name: "lib-1.0.jar", Data: []byte("jar")},
		{Name: "lib-1.0.pom", Data: []byte("<project/>")},
	}
	first, err := buildBundleZip(files)
	require.NoError(t, err)
	second, err := buildBundleZip(files)
	require.NoError(t, err)
	assert.Equal(t, first, second)
}
