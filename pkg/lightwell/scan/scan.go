// Package scan counts possible personal data and secrets in clearinghouse
// intake submissions. It never changes a submission; it only reports how many
// findings of each kind it contains, so the submission can be reviewed.
//
// It looks for four kinds of values in every JSON string value of a submission:
//
//   - email addresses
//   - the username in a home directory path (/home/<u>, /Users/<u>, C:\Users\<u>)
//   - credentials with a published format (vendor tokens, private keys, JWTs,
//     bearer tokens, password= style assignments)
//   - IPv4 and IPv6 addresses, except loopback, link-local, documentation and
//     other ranges that identify nobody, and dotted numbers whose context shows
//     they are versions or section numbers
//
// Every occurrence is counted, so a value that appears three times counts three
// times. A value is counted once even when several rules match it, such as an
// IP address inside a password= value.
//
// The rules are a deliberately small port of the Security TAM sectam-scrubber
// recognizers. Counts are possible findings: they can include false positives,
// and a count of zero does not prove a submission holds no sensitive data.
package scan

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
)

const (
	CategoryEmail      = "email"
	CategoryPathUser   = "path_user"
	CategoryCredential = "credential"
	CategoryIP         = "ip"
)

// skippedKeys are the object keys whose string values are never scanned. They
// hold identifiers that resemble findings but are not: Maven versions in package
// URLs such as name@1.0.Final look like emails, component versions such as
// 1.2.3.4 look like IP addresses, and reproducer_filename names an uploaded file.
var skippedKeys = map[string]struct{}{
	"packageurl":          {},
	"component_name":      {},
	"component_version":   {},
	"reproducer_filename": {},
	"vulnerability_id":    {},
	"cwe":                 {},
	"cvss_vector":         {},
}

// Stats counts the findings in one submission, by category.
type Stats map[string]int

// Total is the number of findings across all categories.
func (s Stats) Total() int {
	total := 0
	for _, n := range s {
		total += n
	}
	return total
}

// JSON counts the findings in every string value of a JSON document. Object
// keys are not scanned. It returns an error when the document is not valid JSON.
func JSON(body []byte) (Stats, error) {
	// The token stream below ends quietly at a truncated document and accepts
	// several top-level values, so validate the whole document first.
	if !json.Valid(body) {
		return nil, errors.New("invalid JSON")
	}
	sc := newScanner()
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()

	var stack []frame
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}

		var parent *frame
		if len(stack) > 0 {
			parent = &stack[len(stack)-1]
		}

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
			if parent != nil && parent.object && parent.expectKey {
				parent.key = v
				parent.expectKey = false
				continue
			}
			skip := parent != nil && parent.object && isSkippedKey(parent.key)
			markValue(parent)
			if !skip {
				sc.mask(v)
			}
		default:
			markValue(parent)
		}
	}
	return sc.stats, nil
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

// Text counts the findings in a single string.
func Text(s string) Stats {
	sc := newScanner()
	sc.mask(s)
	return sc.stats
}

type scanner struct {
	stats Stats
}

func newScanner() *scanner {
	return &scanner{stats: Stats{}}
}

// mask counts the findings in s and returns a working copy of s with each
// finding replaced by a marker such as <EMAIL>. The rules run in order on that
// copy, so a value is counted once: credentials go first, so a token is taken
// whole before an email, path, or IP rule could match part of it. Markers hold
// no character the later rules match. The copy is only used internally and in
// tests to show which spans were found; submissions are never changed.
func (sc *scanner) mask(s string) string {
	s = sc.credentials(s)
	s = sc.emailAddresses(s)
	s = sc.pathUsers(s)
	s = sc.ipAddresses(s)
	return s
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
