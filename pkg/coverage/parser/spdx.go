package parser

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type spdxPURLFields struct {
	Locator    string `json:"referenceLocator"`
	Identifier string `json:"identifier"`
	PackageURL string `json:"packageUrl"`
	Software   string `json:"software_packageUrl"`
}

// purl returns the first non-empty locator/identifier/packageUrl field.
func (f spdxPURLFields) purl() string {
	for _, s := range []string{f.PackageURL, f.Software, f.Locator, f.Identifier} {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

// parseSPDX dispatches SPDX 2/3 JSON or SPDX 2 tag-value (SPDXVersion:) from the start of the stream.
func parseSPDX(r *bufio.Reader) ([]Package, error) {
	start, err := peekSBOMStart(r)
	if err != nil {
		return nil, fmt.Errorf("reading SPDX SBOM: %w", err)
	}
	switch start {
	case '{', '[':
		return parseSPDXJSON(r)
	default:
		peek, _ := r.Peek(256)
		if strings.Contains(string(peek), "SPDXVersion:") {
			return parseSPDXTagValue(r)
		}
		return nil, fmt.Errorf("unsupported SPDX encoding (supported: JSON, tag-value)")
	}
}

// parseSPDXJSON token-walks a 2.x packages list or a 3.x @graph without materializing skipped fields.
func parseSPDXJSON(r io.Reader) ([]Package, error) {
	dec := json.NewDecoder(r)
	pkgs, err := parseSPDXJSONValue(dec, true)
	return pkgs, wrapParse("SPDX JSON", err)
}

// parseSPDXJSONValue walks a JSON value. isRoot is true only for the document so @graph/packages are visited once.
func parseSPDXJSONValue(dec *json.Decoder, isRoot bool) ([]Package, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if tok == nil {
		return nil, nil
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		return nil, nil
	}
	if delim == '{' {
		return parseSPDXJSONObject(dec, isRoot)
	}
	if delim != '[' {
		return nil, fmt.Errorf("expected JSON object or array, got %v", tok)
	}

	var pkgs []Package
	for dec.More() {
		extracted, err := parseSPDXJSONValue(dec, false)
		if err != nil {
			return nil, err
		}
		pkgs = append(pkgs, extracted...)
	}
	_, err = dec.Token()
	return pkgs, err
}

// parseSPDXJSONObject streams one object whose opening '{' was already consumed.
func parseSPDXJSONObject(dec *json.Decoder, isRoot bool) ([]Package, error) {
	var (
		sawPackageType    bool
		sawNonPackageType bool
		purl              string
		pkgs              []Package
	)

	nested := func(d *json.Decoder) error {
		if !isRoot {
			return skipJSONValue(d)
		}
		extracted, err := parseSPDXJSONValue(d, false)
		pkgs = append(pkgs, extracted...)
		return err
	}
	idents := func(d *json.Decoder) error {
		extracted, err := decodeSPDXJSONPurls(d)
		pkgs = append(pkgs, extracted...)
		return err
	}

	err := readJSONObjectFields(dec, compactSPDXKey, map[string]jsonFieldHandler{
		"type": func(d *json.Decoder) error {
			types, err := decodeJSONStringOrArray(d)
			if err != nil {
				return err
			}
			for _, t := range types {
				if isSPDXPackageType(t) {
					sawPackageType = true
				} else if t != "" {
					sawNonPackageType = true
				}
			}
			return nil
		},
		"packageUrl":          func(d *json.Decoder) error { return decodeJSONString(d, &purl) },
		"externalRefs":        idents,
		"externalIdentifier":  idents,
		"externalIdentifiers": idents,
		"packages":            nested,
		"graph":               nested,
		"element":             nested,
		"elements":            nested,
	})
	if err != nil {
		return nil, err
	}

	if sawNonPackageType && !sawPackageType {
		if isRoot {
			return pkgs, nil
		}
		return nil, nil
	}
	return appendFromPURL(pkgs, purl), nil
}

// decodeJSONStringOrArray accepts SPDX 3 type as either "Package" or ["software_Package", "Element"].
func decodeJSONStringOrArray(dec *json.Decoder) ([]string, error) {
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	switch t := v.(type) {
	case string:
		return []string{t}, nil
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out, nil
	default:
		return nil, nil
	}
}

// decodeSPDXJSONPurls reads externalRefs or externalIdentifier as raw JSON (small per package).
func decodeSPDXJSONPurls(dec *json.Decoder) ([]Package, error) {
	var raw json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		return nil, err
	}
	return packagesFromSPDXPURLJSON(raw), nil
}

// packagesFromSPDXPURLJSON extracts maven/pypi purls from a string, object, or array of objects.
func packagesFromSPDXPURLJSON(raw json.RawMessage) []Package {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil
	}

	var pkgs []Package
	switch raw[0] {
	case '"':
		var s string
		if json.Unmarshal(raw, &s) == nil {
			pkgs = appendFromPURL(pkgs, s)
		}
	case '[':
		var fields []spdxPURLFields
		if json.Unmarshal(raw, &fields) == nil {
			for _, f := range fields {
				pkgs = appendFromPURL(pkgs, f.purl())
			}
		}
	case '{':
		var f spdxPURLFields
		if json.Unmarshal(raw, &f) == nil {
			pkgs = appendFromPURL(pkgs, f.purl())
		}
	}
	return pkgs
}

// parseSPDXTagValue reads SPDX tag-value line-by-line, grouping ExternalRef/PackageURL under the current PackageName.
func parseSPDXTagValue(r io.Reader) ([]Package, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	var pkgs []Package
	var current []string
	flush := func() {
		for _, purl := range current {
			pkgs = appendFromPURL(pkgs, purl)
		}
		current = current[:0]
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		switch key {
		case "PackageName", "FileName":
			flush()
		case "ExternalRef":
			fields := strings.Fields(value)
			if len(fields) >= 3 {
				current = append(current, strings.Join(fields[2:], " "))
			}
		case "PackageURL", "packageUrl":
			current = append(current, value)
		}
	}
	flush()
	if err := scanner.Err(); err != nil {
		return nil, wrapParse("SPDX tag-value", err)
	}
	return pkgs, nil
}

// compactSPDXKey maps SPDX 3 aliases (@type, software_packageUrl, RDF URIs) onto a short field name.
func compactSPDXKey(key string) string {
	key = strings.TrimPrefix(key, "@")
	if i := strings.LastIndex(key, "/"); i >= 0 {
		key = key[i+1:]
	}
	key = strings.TrimPrefix(key, "software_")
	key = strings.TrimPrefix(key, "core_")
	return key
}

// isSPDXPackageType is true for SPDX 3 Package types and false for File/Document/etc.
func isSPDXPackageType(typeVal string) bool {
	t := strings.ToLower(strings.TrimSpace(typeVal))
	if t == "" {
		return false
	}
	if i := strings.LastIndexAny(t, "/#"); i >= 0 {
		t = t[i+1:]
	}
	t = strings.TrimPrefix(t, "software_")
	return t == "package"
}
