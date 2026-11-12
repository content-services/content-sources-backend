package sonatype_bridge

import (
	"archive/zip"
	"bytes"
	"fmt"
	"time"
)

// bundleModTime is stamped on every zip entry so that re-processing the same
// artifacts yields a byte-identical archive.
var bundleModTime = time.Date(1980, time.January, 1, 0, 0, 0, 0, time.UTC)

type bundleFile struct {
	Name string
	Data []byte
}

// buildBundleZip writes files, in the given order, into a flat zip archive.
func buildBundleZip(files []bundleFile) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range files {
		w, err := zw.CreateHeader(&zip.FileHeader{
			Name:     f.Name,
			Method:   zip.Deflate,
			Modified: bundleModTime,
		})
		if err != nil {
			return nil, fmt.Errorf("add %s to bundle: %w", f.Name, err)
		}
		if _, err := w.Write(f.Data); err != nil {
			return nil, fmt.Errorf("write %s to bundle: %w", f.Name, err)
		}
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("close bundle: %w", err)
	}
	return buf.Bytes(), nil
}
