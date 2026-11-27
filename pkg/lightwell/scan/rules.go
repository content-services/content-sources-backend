package scan

import (
	"net/netip"
	"regexp"
	"strings"
	"unicode"
)

// credentialRule finds a credential. When group is set, only that submatch is
// the finding and the rest of the match (such as a "Bearer " prefix or a
// "password=" key) is not.
//
// hints are literals at least one of which every match contains. The regex only
// runs on strings that contain a hint, which skips almost all prose: Go cannot
// use its fast literal scan for a pattern that starts with \b. When fold is
// set, hints are lowercase and checked against the lowercased string.
type credentialRule struct {
	re     *regexp.Regexp
	group  int
	accept func(value string) bool
	hints  []string
	fold   bool
}

func literals(hints ...string) []string { return hints }

// credentialMarker replaces a counted credential in the working copy.
const credentialMarker = "<CREDENTIAL>" //nolint:gosec // marker text, not a credential

const (
	pemKeyLabel   = `[A-Z0-9 ]*(?:PRIVATE|PUBLIC) KEY`
	pemCertLabel  = `(?:[A-Z0-9 ]*CERTIFICATE(?: REQUEST)?|X509 CRL|PKCS7|CMS)`
	pgpArmorLabel = `PGP (?:PUBLIC KEY BLOCK|PRIVATE KEY BLOCK|MESSAGE(?:, PART [0-9]+(?:/[0-9]+)?)?)`

	// secretValue is a quoted string or a bare value that stops at whitespace or
	// at punctuation that cannot be part of a bare secret.
	secretValue = `"(?:[^"\\\n]|\\.)*"|'(?:[^'\\\n]|\\.)*'|[^\s` + "`" + `,;)\]}>'"]+`
)

// credentialRules are ported from the sectam-scrubber built-in recognizers.
// Block rules come first so a key or certificate is counted as one finding.
var credentialRules = []credentialRule{
	{hints: literals("-----BEGIN "), re: regexp.MustCompile(`(?s)-----BEGIN ` + pemKeyLabel + `-----.*?-----END ` + pemKeyLabel + `-----`)},
	{hints: literals("-----BEGIN "), re: regexp.MustCompile(`(?s)-----BEGIN ` + pemCertLabel + `-----.*?-----END ` + pemCertLabel + `-----`)},
	{hints: literals("-----BEGIN PGP "), re: regexp.MustCompile(`(?s)-----BEGIN ` + pgpArmorLabel + `-----.*?-----END ` + pgpArmorLabel + `-----`)},
	{hints: literals("-----BEGIN PGP SIGNATURE-----"), re: regexp.MustCompile(`(?s)-----BEGIN PGP SIGNATURE-----.*?-----END PGP SIGNATURE-----`)},
	{hints: literals("---- BEGIN SSH2 "), re: regexp.MustCompile(`(?s)---- BEGIN SSH2 ` + pemKeyLabel + ` ----.*?---- END SSH2 ` + pemKeyLabel + ` ----`)},
	{hints: literals("ssh-rsa"), re: regexp.MustCompile(`ssh-rsa\s+[A-Za-z0-9+/]+={0,2}`)},
	{hints: literals("ssh-ed25519"), re: regexp.MustCompile(`ssh-ed25519\s+[A-Za-z0-9+/]+={0,2}`)},
	{hints: literals("ecdsa-sha2-nistp"), re: regexp.MustCompile(`ecdsa-sha2-nistp(?:256|384|521)\s+[A-Za-z0-9+/]+={0,2}`)},
	// AWS
	{hints: literals("arn:aws"), re: regexp.MustCompile(`\barn:aws(?:-[a-z0-9-]+)?:[a-z0-9-]+:[a-z0-9-]*:[0-9]{12}:[^\s"'<>),]+`)},
	{hints: literals("AKIA", "ASIA"), re: regexp.MustCompile(`\b(?:AKIA|ASIA)[0-9A-Z]{16}\b`)},
	// Vendor token formats. Every prefix is vendor-assigned.
	{hints: literals("ghp_", "gho_", "ghu_", "ghs_", "ghr_", "github_pat_"), re: regexp.MustCompile(`\b(?:gh[pousr]_[A-Za-z0-9]{36,}|github_pat_[A-Za-z0-9]{20,}_[A-Za-z0-9]{50,})\b`)},
	{hints: literals("glpat-", "gldt-", "glrt-", "glft-", "glsoat-", "glptt-"), re: regexp.MustCompile(`\b(?:glpat|gldt|glrt|glft|glsoat|glptt)-[A-Za-z0-9_\-]{20,}\b`)},
	{hints: literals("xox"), re: regexp.MustCompile(`\bxox[baprs]-[A-Za-z0-9-]{10,}\b`)},
	{hints: literals("https://hooks.slack.com/"), re: regexp.MustCompile(`https://hooks\.slack\.com/(?:services|workflows)/[A-Za-z0-9+/]+/[A-Za-z0-9+/]+/[A-Za-z0-9+/]+`)},
	{hints: literals("k_live_", "k_test_"), re: regexp.MustCompile(`\b[sr]k_(?:live|test)_[A-Za-z0-9]{20,}\b`)},
	{hints: literals("AIza"), re: regexp.MustCompile(`\bAIza[A-Za-z0-9_\-]{35}\b`)},
	{hints: literals("sk-ant-"), re: regexp.MustCompile(`\bsk-ant-[A-Za-z0-9_\-]{20,}\b`)},
	{hints: literals("sk-"), re: regexp.MustCompile(`\bsk-(?:proj-|svcacct-|admin-)?[A-Za-z0-9_\-]{20,}\b`)},
	{hints: literals("npm_"), re: regexp.MustCompile(`\bnpm_[A-Za-z0-9]{36}\b`)},
	{hints: literals("pypi-"), re: regexp.MustCompile(`\bpypi-AgEIcHlwaS5vcmc[A-Za-z0-9_\-]{20,}`)},
	{hints: literals("SG."), re: regexp.MustCompile(`\bSG\.[A-Za-z0-9_\-]{16,}\.[A-Za-z0-9_\-]{16,}\b`)},
	{hints: literals("SK"), re: regexp.MustCompile(`\bSK[0-9a-fA-F]{32}\b`)},
	// A JWT payload routinely carries sub, email, and name claims.
	{hints: literals("eyJ"), re: regexp.MustCompile(`\beyJ[A-Za-z0-9_\-]{8,}\.[A-Za-z0-9_\-]{8,}\.[A-Za-z0-9_\-]{8,}`)},
	// "Bearer <token>": the token is the finding, not the scheme. The phrase "bearer
	// tokens" in prose is not a credential.
	{hints: literals("bearer"), fold: true, re: regexp.MustCompile(`(?i)\bbearer\s+([A-Za-z0-9._\-+/~]+=*)`), group: 1, accept: notPlainWord},
	// "password=...", "api_key: ...", `"token": "..."`: the value is the finding, not the key.
	{
		hints: literals("password", "passwd", "pwd", "secret", "token", "key", "connection", "database"),
		fold:  true,
		re: regexp.MustCompile(`(?i)(?:\b|_)(?:password|passwd|pwd|secret|token|api[_-]?key|` +
			`secret[_-]?access[_-]?key|private[_-]?key|client[_-]?secret|` +
			`connection[_-]?string|account[_-]?key|database[_-]?url|` +
			`auth[_-]?token|access[_-]?token|refresh[_-]?token)["']?[ \t]*[:=][ \t]*(` + secretValue + `)`),
		group:  1,
		accept: looksLikeSecretValue,
	},
}

func containsAny(s string, hints []string) bool {
	for _, hint := range hints {
		if strings.Contains(s, hint) {
			return true
		}
	}
	return false
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

func (sc *scanner) credentials(s string) string {
	lower := ""
	for _, rule := range credentialRules {
		target := s
		if rule.fold {
			if lower == "" {
				lower = strings.ToLower(s)
			}
			target = lower
		}
		if !containsAny(target, rule.hints) {
			continue
		}
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
			lower = "" // s changes below, so the cached lowercase copy is stale
			sc.stats[CategoryCredential]++
			// A quoted assignment value keeps its quotes in the working copy.
			if rule.group > 0 && len(value) >= 2 && (value[0] == '"' || value[0] == '\'') {
				return value[:1] + credentialMarker + value[len(value)-1:], true
			}
			return credentialMarker, true
		})
	}
	return s
}

// emailPattern matches a local part in any script, an at sign (ASCII or the
// full-width form CJK input methods type), and a domain ending in a letter TLD.
// Group 1 is the domain.
var emailPattern = regexp.MustCompile(`[\p{L}\p{M}\p{N}._%+\-]+[@\x{FF20}]([A-Za-z0-9.\-]+\.[A-Za-z]{2,})`)

func (sc *scanner) emailAddresses(s string) string {
	if !strings.ContainsRune(s, '@') && !strings.ContainsRune(s, '\uFF20') {
		return s
	}
	matches := emailPattern.FindAllStringSubmatchIndex(s, -1)
	return replaceSpans(s, matches, func(m []int) (string, bool) {
		if versionLikeDomain(s[m[2]:m[3]]) || scpRemote(s, m[1]) {
			return "", false
		}
		sc.stats[CategoryEmail]++
		return "<EMAIL>", true
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

func (sc *scanner) pathUsers(s string) string {
	if !strings.Contains(s, "/home/") && !strings.Contains(s, "/Users/") && !strings.Contains(strings.ToLower(s), "users") {
		return s
	}
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
			sc.stats[CategoryPathUser]++
			return "<USER>", true
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

// IP address rules, ported from the sectam-scrubber ipv4 and ipv6 recognizers.
var (
	// ipv4Pattern has no \b anchors because Go treats '_' as a word character,
	// which would hide host_10.20.30.40. ipv4Boundaries checks the edges instead.
	ipv4Pattern = regexp.MustCompile(`(?:[0-9]{1,3}\.){3}[0-9]{1,3}`)

	// ipv6Pattern is a broad IPv6 shape that netip validates. Group 1 is the
	// address. The prefix (start, or a character that is neither a letter, a
	// digit, nor a colon) keeps C++ scopes such as std::vector out.
	ipv6Pattern = func() *regexp.Regexp {
		re := regexp.MustCompile(`(?:^|[^0-9A-Za-z:])(` +
			`(?:[A-Fa-f0-9]{1,4}:){2,7}[A-Fa-f0-9]{1,4}` +
			`|(?:[A-Fa-f0-9]{1,4}:){1,7}:(?:[A-Fa-f0-9]{1,4})?` +
			`|(?:[A-Fa-f0-9]{1,4}:){1,6}(?::[A-Fa-f0-9]{1,4})+` +
			`|::(?:[A-Fa-f0-9]{1,4}:)*[A-Fa-f0-9]{1,4}` +
			`)`)
		re.Longest()
		return re
	}()

	// packageNameBeforeVersion matches a hyphenated package name directly before
	// a version, as in daon-http-digest-auth-5.0.1.0.
	packageNameBeforeVersion = regexp.MustCompile(`(?:^|[^0-9A-Za-z._@-])[A-Za-z][0-9A-Za-z]*(?:[-_][A-Za-z][0-9A-Za-z]*)+[-_]$`)

	// nonIdentifyingPrefixes are documentation, benchmarking, and reserved
	// ranges that identify nobody. Private ranges (10/8, 172.16/12,
	// 192.168/16, fc00::/7) are deliberately absent: internal addressing is
	// exactly what a submission should not leak.
	nonIdentifyingPrefixes = []netip.Prefix{
		netip.MustParsePrefix("192.0.2.0/24"),    // RFC 5737 TEST-NET-1
		netip.MustParsePrefix("198.51.100.0/24"), // RFC 5737 TEST-NET-2
		netip.MustParsePrefix("203.0.113.0/24"),  // RFC 5737 TEST-NET-3
		netip.MustParsePrefix("198.18.0.0/15"),   // RFC 2544 benchmarking
		netip.MustParsePrefix("240.0.0.0/4"),     // reserved, includes broadcast
		netip.MustParsePrefix("2001:db8::/32"),   // RFC 3849 documentation
	}
)

// purlStops end a package URL in running text.
const purlStops = " \t\r\n\"'`<>,;"

// versionContextWords precede a dotted number that is a section reference, as
// in "section 5.2.3.10", or a version, as in "version 1.2.3.4".
var versionContextWords = []string{"section", "clause", "appendix", "chapter", "draft", "version", "release"}

func (sc *scanner) ipAddresses(s string) string {
	if strings.Contains(s, ".") {
		s = replaceSpans(s, ipv4Pattern.FindAllStringIndex(s, -1), func(m []int) (string, bool) {
			addr, err := netip.ParseAddr(s[m[0]:m[1]])
			if err != nil || !ipv4Boundaries(s, m[0], m[1]) || nonIdentifyingIP(addr) || dottedQuadIsNotAnAddress(s, m[0], m[1]) {
				return "", false
			}
			sc.stats[CategoryIP]++
			return "<IP>", true
		})
	}
	if !strings.Contains(s, ":") {
		return s
	}
	matches := ipv6Pattern.FindAllStringSubmatchIndex(s, -1)
	spans := make([][]int, 0, len(matches))
	for _, m := range matches {
		spans = append(spans, []int{m[2], m[3]})
	}
	return replaceSpans(s, spans, func(m []int) (string, bool) {
		text := s[m[0]:m[1]]
		if text == "::" {
			return "", false
		}
		addr, err := netip.ParseAddr(text)
		if err != nil || !addr.Is6() || nonIdentifyingIP(addr) {
			return "", false
		}
		sc.stats[CategoryIP]++
		return "<IP>", true
	})
}

// nonIdentifyingIP reports whether an address identifies nobody: loopback,
// link-local (including the 169.254.169.254 cloud metadata address SSRF
// reports quote), unspecified, multicast, and documentation or reserved ranges.
func nonIdentifyingIP(addr netip.Addr) bool {
	if addr.IsLoopback() || addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() ||
		addr.IsUnspecified() || addr.IsMulticast() {
		return true
	}
	for _, prefix := range nonIdentifyingPrefixes {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

// ipv4Boundaries reports whether a dotted quad stands apart from the text
// around it: no letter or digit touches either end, so v1.2.3.4 and 1234.1.1.1
// are not addresses while host_10.20.30.40 and 10.1.1.7-10.1.1.8 are.
func ipv4Boundaries(s string, start, end int) bool {
	return (start == 0 || !isAlnum(s[start-1])) && (end == len(s) || !isAlnum(s[end]))
}

// dottedQuadIsNotAnAddress reports whether the text around a dotted quad shows
// it is a version, section number, or OID rather than an address. It is decided
// from the adjacent text, because a value such as 5.2.3.10 is a valid address
// on its own. Anything not positively identified is treated as an address.
func dottedQuadIsNotAnAddress(s string, start, end int) bool {
	// Part of a longer dotted run (1.3.6.1.4.1), which is a version or an OID.
	if end+1 < len(s) && s[end] == '.' && isDigit(s[end+1]) {
		return true
	}
	if start >= 2 && s[start-1] == '.' && isDigit(s[start-2]) {
		return true
	}
	before := s[max(0, start-40):start]
	if packageNameBeforeVersion.MatchString(before) {
		return true
	}
	// The version of a package URL on the same line, pkg:maven/a/b@1.2.3.4, or
	// that version repeated later on the line.
	line := s[strings.LastIndexByte(s[:start], '\n')+1 : start]
	if scheme := strings.LastIndex(line, "pkg:"); scheme >= 0 {
		purl := line[scheme:]
		if strings.HasSuffix(purl, "@") && !strings.ContainsAny(purl, purlStops) {
			return true
		}
		if strings.Contains(line, "@"+s[start:end]) {
			return true
		}
	}
	trimmed := strings.TrimRight(before, " \t")
	if strings.HasSuffix(trimmed, "§") || strings.HasSuffix(trimmed, "#") {
		return true
	}
	lower := strings.ToLower(trimmed)
	for _, word := range versionContextWords {
		if strings.HasSuffix(lower, word) {
			return true
		}
	}
	return false
}

func isAlnum(c byte) bool {
	return isDigit(c) || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}
