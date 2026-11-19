// Package redact removes personally identifiable information from clearinghouse
// intake submissions before they are parsed or stored.
//
// It replaces three kinds of values in every JSON string value of a submission:
//
//   - email addresses, with <EMAIL_n>
//   - the username in a home directory path (/home/<u>, /Users/<u>, C:\Users\<u>), with <USER_n>
//   - credentials with a published format (vendor tokens, private keys, JWTs,
//     bearer tokens, password= style assignments), with <CREDENTIAL>
//
// The same email or username gets the same number everywhere in one submission.
// Numbering restarts for every submission and no mapping is kept, so redacted
// submissions cannot be linked to each other.
//
// The rules are a deliberately small port of the Security TAM sectam-scrubber
// recognizers. Redaction is best-effort risk reduction, not anonymization.
package redact

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

const (
	CategoryEmail      = "email"
	CategoryPathUser   = "path_user"
	CategoryCredential = "credential"

	credentialPlaceholder = "<CREDENTIAL>" //nolint:gosec // placeholder text, not a credential
)

// skippedKeys are the object keys whose string values are never redacted. They
// hold identifiers that must survive unchanged: package URLs are parsed into the
// package list (and Maven versions such as name@1.0.Final look like emails), and
// reproducer_filename must match the name of an uploaded file.
var skippedKeys = map[string]struct{}{
	"packageurl":          {},
	"reproducer_filename": {},
	"vulnerability_id":    {},
	"cwe":                 {},
	"cvss_vector":         {},
}

// Stats counts the replacements made in one submission, by category.
type Stats map[string]int

// Total is the number of replacements across all categories.
func (s Stats) Total() int {
	total := 0
	for _, n := range s {
		total += n
	}
	return total
}

// JSON redacts every string value in a JSON document and returns the result.
// Object keys, numbers, formatting, and every string that needs no redaction are
// kept byte for byte. It returns an error when the document is not valid JSON.
func JSON(body []byte) ([]byte, Stats, error) {
	// The token stream below ends quietly at a truncated document and accepts
	// several top-level values, so validate the whole document first.
	if !json.Valid(body) {
		return nil, nil, errors.New("invalid JSON")
	}
	r := newRedactor()
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()

	var stack []frame
	var out bytes.Buffer
	copied := 0

	for {
		start := dec.InputOffset()
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, nil, err
		}
		end := dec.InputOffset()

		var parent *frame
		if len(stack) > 0 {
			parent = &stack[len(stack)-1]
		}
		isKey := parent != nil && parent.object && parent.expectKey

		switch v := tok.(type) {
		case json.Delim:
			switch v {
			case '{':
				markValue(parent)
				stack = append(stack, frame{object: true, expectKey: true})
			case '[':
				markValue(parent)
				stack = append(stack, frame{})
			default:
				stack = stack[:len(stack)-1]
			}
		case string:
			if isKey {
				parent.key = v
				parent.expectKey = false
				continue
			}
			skip := parent != nil && parent.object && isSkippedKey(parent.key)
			markValue(parent)
			if skip {
				continue
			}
			redacted := r.text(v)
			if redacted == v {
				continue
			}
			// The span between offsets holds separators, whitespace, and the
			// string literal; separators are never quotes, so the first quote
			// opens the literal.
			quote := bytes.IndexByte(body[start:end], '"')
			if quote < 0 {
				return nil, nil, fmt.Errorf("cannot locate string literal at offset %d", start)
			}
			literalStart := int(start) + quote
			encoded, err := encodeString(redacted)
			if err != nil {
				return nil, nil, err
			}
			out.Write(body[copied:literalStart])
			out.Write(encoded)
			copied = int(end)
		default:
			markValue(parent)
		}
	}
	if copied == 0 {
		return body, r.stats, nil
	}
	out.Write(body[copied:])
	return out.Bytes(), r.stats, nil
}

// frame is an open JSON object or array.
type frame struct {
	object    bool
	expectKey bool
	key       string
}

// markValue records that a value was read in parent, so in an object the next
// string is a key.
func markValue(parent *frame) {
	if parent != nil && parent.object {
		parent.expectKey = true
	}
}

func isSkippedKey(key string) bool {
	_, ok := skippedKeys[strings.ToLower(key)]
	return ok
}

// encodeString encodes s as a JSON string without escaping <, >, and &, so
// placeholders stay readable.
func encodeString(s string) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(s); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// Text redacts a single string.
func Text(s string) (string, Stats) {
	r := newRedactor()
	return r.text(s), r.stats
}

type redactor struct {
	emails map[string]int
	users  map[string]int
	stats  Stats
}

func newRedactor() *redactor {
	return &redactor{emails: map[string]int{}, users: map[string]int{}, stats: Stats{}}
}

// text applies the rules in order. Credentials go first so a token is replaced
// whole before an email or path rule could match part of it. Placeholders hold
// no character the later rules match, so a replacement is never redacted twice.
func (r *redactor) text(s string) string {
	s = r.credentials(s)
	s = r.emailAddresses(s)
	s = r.pathUsers(s)
	return s
}

func (r *redactor) placeholder(seen map[string]int, prefix, key string) string {
	n, ok := seen[key]
	if !ok {
		n = len(seen) + 1
		seen[key] = n
	}
	return "<" + prefix + "_" + strconv.Itoa(n) + ">"
}

// replaceSpans rebuilds s with each [start, end) span replaced by the result of
// repl, skipping spans for which repl returns ok false. Spans must be ordered and
// must not overlap.
func replaceSpans(s string, spans [][]int, repl func(span []int) (string, bool)) string {
	if len(spans) == 0 {
		return s
	}
	var b strings.Builder
	last := 0
	for _, span := range spans {
		replacement, ok := repl(span)
		if !ok {
			continue
		}
		b.WriteString(s[last:span[0]])
		b.WriteString(replacement)
		last = span[1]
	}
	if last == 0 {
		return s
	}
	b.WriteString(s[last:])
	return b.String()
}
