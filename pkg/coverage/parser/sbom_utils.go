package parser

import (
	"bufio"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
	"unicode"
)

// skipWhitespace consumes leading spaces so the next Peek sees '{', '<', or a tag-value start.
func skipWhitespace(r *bufio.Reader) error {
	for {
		b, err := r.Peek(1)
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		if !unicode.IsSpace(rune(b[0])) {
			return nil
		}
		if _, err := r.Discard(1); err != nil {
			return err
		}
	}
}

// discardUTF8BOM drops a leading EF BB BF so JSON/XML parsers are not tripped up by a byte-order mark.
func discardUTF8BOM(r *bufio.Reader) {
	b, err := r.Peek(3)
	if err == nil && len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		_, _ = r.Discard(3)
	}
}

// peekSBOMStart returns the first non-space byte used to pick JSON vs XML vs tag-value.
func peekSBOMStart(r *bufio.Reader) (byte, error) {
	if err := skipWhitespace(r); err != nil {
		return 0, err
	}
	b, err := r.Peek(1)
	if err != nil {
		return 0, err
	}
	return b[0], nil
}

// sniffSBOMFormat peeks at the start of the file (without consuming it) to tell CycloneDX from SPDX.
func sniffSBOMFormat(r *bufio.Reader) (string, error) {
	peek, err := r.Peek(8192)
	if err != nil && err != io.EOF && len(peek) == 0 {
		return "", err
	}
	s := strings.TrimSpace(string(peek))
	lower := strings.ToLower(s)

	switch {
	case strings.Contains(lower, `"bomformat"`) ||
		strings.Contains(lower, "cyclonedx.org") ||
		strings.Contains(lower, "<bom"):
		return FormatCycloneDX, nil
	case strings.Contains(s, "spdxVersion") ||
		strings.Contains(s, "SPDXVersion") ||
		strings.Contains(lower, "spdx.org") ||
		strings.Contains(lower, "spdxid") ||
		strings.Contains(lower, `"@graph"`):
		return FormatSPDX, nil
	}
	return "", nil
}

// forEachXMLStart walks XML start elements and lets the caller DecodeElement or Skip each one.
func forEachXMLStart(r io.Reader, handle func(dec *xml.Decoder, se xml.StartElement) error) error {
	dec := xml.NewDecoder(r)
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		if err := handle(dec, se); err != nil {
			return err
		}
	}
}

// appendFromPURL adds the package if the purl is a supported maven/pypi identity; otherwise leaves pkgs unchanged.
func appendFromPURL(pkgs []Package, purl string) []Package {
	if pkg := parsePURL(strings.TrimSpace(purl)); pkg != nil {
		return append(pkgs, *pkg)
	}
	return pkgs
}

// hasAnySuffix reports whether s ends with any of the given suffixes.
func hasAnySuffix(s string, suffixes ...string) bool {
	for _, suffix := range suffixes {
		if strings.HasSuffix(s, suffix) {
			return true
		}
	}
	return false
}

// wrapParse prefixes err with the SBOM serialization being parsed.
func wrapParse(kind string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("parsing %s: %w", kind, err)
}

type jsonFieldHandler func(dec *json.Decoder) error

// readJSONObject streams one JSON object, running handlers for known keys and discarding the rest.
func readJSONObject(dec *json.Decoder, handlers map[string]jsonFieldHandler) error {
	return readJSONObjectKeys(dec, nil, handlers)
}

// readJSONObjectKeys is readJSONObject with an optional key rewrite (used for SPDX 3 aliases).
func readJSONObjectKeys(dec *json.Decoder, mapKey func(string) string, handlers map[string]jsonFieldHandler) error {
	if err := consumeJSONDelim(dec, '{'); err != nil {
		return err
	}
	return readJSONObjectFields(dec, mapKey, handlers)
}

// consumeJSONDelim reads the next token and requires it to be the given delimiter.
func consumeJSONDelim(dec *json.Decoder, want json.Delim) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if tok == nil {
		if want == '{' {
			return nil
		}
		return fmt.Errorf("expected JSON delimiter %q, got null", want)
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != want {
		return fmt.Errorf("expected JSON delimiter %q, got %v", want, tok)
	}
	return nil
}

// readJSONObjectFields walks keys of an object whose opening '{' was already consumed.
func readJSONObjectFields(dec *json.Decoder, mapKey func(string) string, handlers map[string]jsonFieldHandler) error {
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return err
		}
		key, ok := keyTok.(string)
		if !ok {
			return fmt.Errorf("expected object key, got %v", keyTok)
		}
		if mapKey != nil {
			key = mapKey(key)
		}
		if h, ok := handlers[key]; ok {
			if err := h(dec); err != nil {
				return err
			}
			continue
		}
		if err := skipJSONValue(dec); err != nil {
			return err
		}
	}
	_, err := dec.Token()
	return err
}

// forEachJSONArray streams each element of a JSON array (or no-ops on null).
func forEachJSONArray(dec *json.Decoder, fn func() error) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if tok == nil {
		return nil
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != '[' {
		return fmt.Errorf("expected JSON array, got %v", tok)
	}

	for dec.More() {
		if err := fn(); err != nil {
			return err
		}
	}

	_, err = dec.Token()
	return err
}

// appendParsedJSONArray parses each array element into packages and appends them to pkgs.
func appendParsedJSONArray(dec *json.Decoder, pkgs *[]Package, parse func(*json.Decoder) ([]Package, error)) error {
	return forEachJSONArray(dec, func() error {
		extracted, err := parse(dec)
		*pkgs = append(*pkgs, extracted...)
		return err
	})
}

// skipJSONValue consumes the next JSON value token-by-token so large skipped fields stay off the heap.
func skipJSONValue(dec *json.Decoder) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		return nil
	}

	switch delim {
	case '{':
		for dec.More() {
			if _, err := dec.Token(); err != nil {
				return err
			}
			if err := skipJSONValue(dec); err != nil {
				return err
			}
		}
		_, err = dec.Token()
		return err
	case '[':
		for dec.More() {
			if err := skipJSONValue(dec); err != nil {
				return err
			}
		}
		_, err = dec.Token()
		return err
	default:
		return fmt.Errorf("unexpected JSON delimiter %v", delim)
	}
}

// decodeJSONString reads the next value and stores it only when it is actually a string.
func decodeJSONString(dec *json.Decoder, dst *string) error {
	var v any
	if err := dec.Decode(&v); err != nil {
		return err
	}
	if s, ok := v.(string); ok {
		*dst = s
	}
	return nil
}
