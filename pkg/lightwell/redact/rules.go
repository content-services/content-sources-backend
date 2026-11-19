package redact

import (
	"regexp"
	"strings"
	"unicode"
)

// credentialRule replaces a credential. When group is set, only that submatch is
// replaced and the rest of the match (such as a "Bearer " prefix or a
// "password=" key) is kept.
type credentialRule struct {
	re     *regexp.Regexp
	group  int
	accept func(value string) bool
}

const (
	pemKeyLabel   = `[A-Z0-9 ]*(?:PRIVATE|PUBLIC) KEY`
	pemCertLabel  = `(?:[A-Z0-9 ]*CERTIFICATE(?: REQUEST)?|X509 CRL|PKCS7|CMS)`
	pgpArmorLabel = `PGP (?:PUBLIC KEY BLOCK|PRIVATE KEY BLOCK|MESSAGE(?:, PART [0-9]+(?:/[0-9]+)?)?)`

	// secretValue is a quoted string or a bare value that stops at whitespace or
	// at punctuation that cannot be part of a bare secret.
	secretValue = `"(?:[^"\\\n]|\\.)*"|'(?:[^'\\\n]|\\.)*'|[^\s` + "`" + `,;)\]}>'"]+`
)

// credentialRules are ported from the sectam-scrubber built-in recognizers.
// Block rules come first so a key or certificate is replaced whole.
var credentialRules = []credentialRule{
	{re: regexp.MustCompile(`(?s)-----BEGIN ` + pemKeyLabel + `-----.*?-----END ` + pemKeyLabel + `-----`)},
	{re: regexp.MustCompile(`(?s)-----BEGIN ` + pemCertLabel + `-----.*?-----END ` + pemCertLabel + `-----`)},
	{re: regexp.MustCompile(`(?s)-----BEGIN ` + pgpArmorLabel + `-----.*?-----END ` + pgpArmorLabel + `-----`)},
	{re: regexp.MustCompile(`(?s)-----BEGIN PGP SIGNATURE-----.*?-----END PGP SIGNATURE-----`)},
	{re: regexp.MustCompile(`(?s)---- BEGIN SSH2 ` + pemKeyLabel + ` ----.*?---- END SSH2 ` + pemKeyLabel + ` ----`)},
	{re: regexp.MustCompile(`ssh-rsa\s+[A-Za-z0-9+/]+={0,2}`)},
	{re: regexp.MustCompile(`ssh-ed25519\s+[A-Za-z0-9+/]+={0,2}`)},
	{re: regexp.MustCompile(`ecdsa-sha2-nistp(?:256|384|521)\s+[A-Za-z0-9+/]+={0,2}`)},
	// AWS
	{re: regexp.MustCompile(`\barn:aws(?:-[a-z0-9-]+)?:[a-z0-9-]+:[a-z0-9-]*:[0-9]{12}:[^\s"'<>),]+`)},
	{re: regexp.MustCompile(`\b(?:AKIA|ASIA)[0-9A-Z]{16}\b`)},
	// Vendor token formats. Every prefix is vendor-assigned.
	{re: regexp.MustCompile(`\b(?:gh[pousr]_[A-Za-z0-9]{36,}|github_pat_[A-Za-z0-9]{20,}_[A-Za-z0-9]{50,})\b`)},
	{re: regexp.MustCompile(`\b(?:glpat|gldt|glrt|glft|glsoat|glptt)-[A-Za-z0-9_\-]{20,}\b`)},
	{re: regexp.MustCompile(`\bxox[baprs]-[A-Za-z0-9-]{10,}\b`)},
	{re: regexp.MustCompile(`https://hooks\.slack\.com/(?:services|workflows)/[A-Za-z0-9+/]+/[A-Za-z0-9+/]+/[A-Za-z0-9+/]+`)},
	{re: regexp.MustCompile(`\b[sr]k_(?:live|test)_[A-Za-z0-9]{20,}\b`)},
	{re: regexp.MustCompile(`\bAIza[A-Za-z0-9_\-]{35}\b`)},
	{re: regexp.MustCompile(`\bsk-ant-[A-Za-z0-9_\-]{20,}\b`)},
	{re: regexp.MustCompile(`\bsk-(?:proj-|svcacct-|admin-)?[A-Za-z0-9_\-]{20,}\b`)},
	{re: regexp.MustCompile(`\bnpm_[A-Za-z0-9]{36}\b`)},
	{re: regexp.MustCompile(`\bpypi-AgEIcHlwaS5vcmc[A-Za-z0-9_\-]{20,}`)},
	{re: regexp.MustCompile(`\bSG\.[A-Za-z0-9_\-]{16,}\.[A-Za-z0-9_\-]{16,}\b`)},
	{re: regexp.MustCompile(`\bSK[0-9a-fA-F]{32}\b`)},
	// A JWT payload routinely carries sub, email, and name claims.
	{re: regexp.MustCompile(`\beyJ[A-Za-z0-9_\-]{8,}\.[A-Za-z0-9_\-]{8,}\.[A-Za-z0-9_\-]{8,}`)},
	// "Bearer <token>": keep the scheme, replace the token. The phrase "bearer
	// tokens" in prose is not a credential.
	{re: regexp.MustCompile(`(?i)\bbearer\s+([A-Za-z0-9._\-+/~]+=*)`), group: 1, accept: notPlainWord},
	// "password=...", "api_key: ...", `"token": "..."`: keep the key, replace the value.
	{
		re: regexp.MustCompile(`(?i)(?:\b|_)(?:password|passwd|pwd|secret|token|api[_-]?key|` +
			`secret[_-]?access[_-]?key|private[_-]?key|client[_-]?secret|` +
			`connection[_-]?string|account[_-]?key|database[_-]?url|` +
			`auth[_-]?token|access[_-]?token|refresh[_-]?token)["']?[ \t]*[:=][ \t]*(` + secretValue + `)`),
		group:  1,
		accept: looksLikeSecretValue,
	},
}

// notPlainWord rejects a bearer "token" that is an ordinary word.
func notPlainWord(token string) bool {
	return !isLetters(strings.TrimRight(token, ".,;:!?)"))
}

// looksLikeSecretValue rejects assignment values that are prose, code, a path, or
// a placeholder rather than a secret, so write-ups keep their meaning:
// "password: required", `token = getToken()`, "PWD=/home/x", "secret=${SECRET}".
func looksLikeSecretValue(value string) bool {
	inner := value
	if len(value) >= 2 && (value[0] == '"' || value[0] == '\'') {
		inner = value[1 : len(value)-1]
		if inner == "" {
			return false
		}
	} else if isLetters(value) {
		return false
	}
	if strings.ContainsAny(inner[:1], "$<{%*/~") {
		return false
	}
	return !strings.Contains(inner, "(")
}

func isLetters(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if !unicode.IsLetter(c) {
			return false
		}
	}
	return true
}

func (r *redactor) credentials(s string) string {
	for _, rule := range credentialRules {
		matches := rule.re.FindAllStringSubmatchIndex(s, -1)
		if len(matches) == 0 {
			continue
		}
		spans := make([][]int, 0, len(matches))
		for _, m := range matches {
			start, end := m[2*rule.group], m[2*rule.group+1]
			if start < 0 {
				continue
			}
			spans = append(spans, []int{start, end})
		}
		s = replaceSpans(s, spans, func(span []int) (string, bool) {
			value := s[span[0]:span[1]]
			if rule.accept != nil && !rule.accept(value) {
				return "", false
			}
			r.stats[CategoryCredential]++
			// A quoted assignment value keeps its quotes.
			if rule.group > 0 && len(value) >= 2 && (value[0] == '"' || value[0] == '\'') {
				return value[:1] + credentialPlaceholder + value[len(value)-1:], true
			}
			return credentialPlaceholder, true
		})
	}
	return s
}

// emailPattern matches a local part in any script, an at sign (ASCII or the
// full-width form CJK input methods type), and a domain ending in a letter TLD.
// Group 1 is the domain.
var emailPattern = regexp.MustCompile(`[\p{L}\p{M}\p{N}._%+\-]+[@\x{FF20}]([A-Za-z0-9.\-]+\.[A-Za-z]{2,})`)

func (r *redactor) emailAddresses(s string) string {
	matches := emailPattern.FindAllStringSubmatchIndex(s, -1)
	return replaceSpans(s, matches, func(m []int) (string, bool) {
		if versionLikeDomain(s[m[2]:m[3]]) || scpRemote(s, m[1]) {
			return "", false
		}
		r.stats[CategoryEmail]++
		return r.placeholder(r.emails, "EMAIL", strings.ToLower(s[m[0]:m[1]])), true
	})
}

// versionLikeDomain reports whether a "domain" is a version string, as in
// spring-core@5.3.RELEASE or name@1.0.Final: two or more all-digit labels.
func versionLikeDomain(domain string) bool {
	digits := 0
	for _, label := range strings.Split(domain, ".") {
		if label != "" && strings.Trim(label, "0123456789") == "" {
			digits++
		}
	}
	return digits >= 2
}

// scpRemote reports whether the match ending at end is an SCP-style remote such
// as git@github.com:org/repo, which is a URL rather than an email address.
func scpRemote(s string, end int) bool {
	return end+1 < len(s) && s[end] == ':' && !unicode.IsSpace(rune(s[end+1]))
}

// usernameChars are the characters a home directory username may hold here. It
// stops at path separators, whitespace, quotes, and syntax used around paths in
// shells, JSON, passwd files, and PATH lists. Placeholders such as $USER,
// ${USER}, %USERNAME%, <user>, and {user} start with an excluded character and
// so never match.
const usernameChars = `[^\s/\\"'` + "`" + `\[\];|=*?<>,:(){}$%]+`

var pathUserPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(/home/|/Users/)(` + usernameChars + `)`),
	regexp.MustCompile(`(?i)(\b[a-z]:[\\/]+users[\\/]+)(` + usernameChars + `)`),
}

// sharedHomeDirs are home directories that belong to no person.
var sharedHomeDirs = map[string]struct{}{
	"shared":  {},
	"public":  {},
	"default": {},
}

// urlPattern finds URLs, whose paths are left alone. file:// URLs are local
// paths and are not excluded.
var urlPattern = regexp.MustCompile(`[A-Za-z][A-Za-z0-9+.\-]*://[^\s"'<>]+`)

func (r *redactor) pathUsers(s string) string {
	for _, re := range pathUserPatterns {
		matches := re.FindAllStringSubmatchIndex(s, -1)
		if len(matches) == 0 {
			continue
		}
		urls := remoteURLSpans(s)
		spans := make([][]int, 0, len(matches))
		for _, m := range matches {
			start := m[4]
			end := start + len(strings.TrimRight(s[start:m[5]], ".!"))
			if end == start || insideAny(start, urls) {
				continue
			}
			if _, shared := sharedHomeDirs[strings.ToLower(s[start:end])]; shared {
				continue
			}
			spans = append(spans, []int{start, end})
		}
		s = replaceSpans(s, spans, func(span []int) (string, bool) {
			r.stats[CategoryPathUser]++
			return r.placeholder(r.users, "USER", strings.ToLower(s[span[0]:span[1]])), true
		})
	}
	return s
}

func remoteURLSpans(s string) [][]int {
	var spans [][]int
	for _, span := range urlPattern.FindAllStringIndex(s, -1) {
		if !strings.HasPrefix(strings.ToLower(s[span[0]:span[1]]), "file://") {
			spans = append(spans, span)
		}
	}
	return spans
}

func insideAny(pos int, spans [][]int) bool {
	for _, span := range spans {
		if pos >= span[0] && pos < span[1] {
			return true
		}
	}
	return false
}
