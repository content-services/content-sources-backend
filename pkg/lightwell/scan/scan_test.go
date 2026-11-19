package scan

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTextEmails(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"plain", "contact jsmith@corp.com now", "contact <EMAIL> now"},
		{"example domain", "attacker@example.com", "<EMAIL>"},
		{"repeated address", "a@x.com b@y.org A@X.COM", "<EMAIL> <EMAIL> <EMAIL>"},
		{"unicode local part", "ivan.petrov@corp.ru and josé@corp.es", "<EMAIL> and <EMAIL>"},
		{"full-width at sign", "tanaka＠corp.jp", "<EMAIL>"},
		{"mailto", "mailto:sec@corp.com", "mailto:<EMAIL>"},
		{"digit-label provider", "user@163.com", "<EMAIL>"},
		{"maven version", "spring-core@5.3.RELEASE and name@1.0.Final", "spring-core@5.3.RELEASE and name@1.0.Final"},
		{"npm version", "lodash@4.17.21", "lodash@4.17.21"},
		{"scp remote", "git clone git@github.com:org/repo.git", "git clone git@github.com:org/repo.git"},
		{"sentence colon", "write to a@corp.com: thanks", "write to <EMAIL>: thanks"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := masked(tc.in)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestTextPathUsers(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"home", "/home/jsmith/app/config.yml", "/home/<USER>/app/config.yml"},
		{"macos", "/Users/jsmith/Library/Logs", "/Users/<USER>/Library/Logs"},
		{"windows", `C:\Users\jsmith\AppData\Local`, `C:\Users\<USER>\AppData\Local`},
		{"windows escaped", `C:\\Users\\jsmith\\AppData`, `C:\\Users\\<USER>\\AppData`},
		{"windows forward slash", "c:/users/jsmith/x", "c:/users/<USER>/x"},
		{"bare home", "files live in /home/jsmith.", "files live in /home/<USER>."},
		{"repeated user", "/home/jsmith/a /Users/kbrown/b /home/jsmith/c", "/home/<USER>/a /Users/<USER>/b /home/<USER>/c"},
		{"passwd line", "jsmith:x:1000:1000::/home/jsmith:/bin/bash", "jsmith:x:1000:1000::/home/<USER>:/bin/bash"},
		{"PATH list", "PATH=/home/kbrown/bin:/usr/bin", "PATH=/home/<USER>/bin:/usr/bin"},
		{"markdown link", "[notes](/home/kbrown)", "[notes](/home/<USER>)"},
		{"csv row", "web01,/home/jli,/opt/app", "web01,/home/<USER>,/opt/app"},
		{"dotted name", "/home/dan.oneil/.profile", "/home/<USER>/.profile"},
		{"file url", "file:///home/jsmith/x", "file:///home/<USER>/x"},
		{"system paths untouched", "/etc/passwd /opt/app/bin /usr/lib64/libssl.so node_modules/x/y.js", "/etc/passwd /opt/app/bin /usr/lib64/libssl.so node_modules/x/y.js"},
		{"placeholders untouched", "/home/$USER/x /home/${USER}/x C:\\Users\\%USERNAME%\\x /home/<user>/x /home/{user}/x", "/home/$USER/x /home/${USER}/x C:\\Users\\%USERNAME%\\x /home/<user>/x /home/{user}/x"},
		{"shared dirs untouched", "/Users/Shared/x C:\\Users\\Public\\x", "/Users/Shared/x C:\\Users\\Public\\x"},
		{"remote url untouched", "https://github.com/org/repo/blob/main/home/jsmith/x", "https://github.com/org/repo/blob/main/home/jsmith/x"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := masked(tc.in)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestTextCredentials(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"github classic", "token ghp_" + repeat("a", 36) + " leaked", "token <CREDENTIAL> leaked"},
		{"github fine-grained", "github_pat_" + repeat("A", 22) + "_" + repeat("b", 59), "<CREDENTIAL>"},
		{"gitlab", "glpat-" + repeat("x", 20), "<CREDENTIAL>"},
		{"aws access key", "AKIAIOSFODNN7EXAMPLE", "<CREDENTIAL>"},
		{"aws temp key", "ASIAIOSFODNN7EXAMPLE", "<CREDENTIAL>"},
		{"aws arn", "role arn:aws:iam::123456789012:role/Admin, then", "role <CREDENTIAL>, then"},
		{"slack token", "xoxb-1234567890-abcdef", "<CREDENTIAL>"},
		{"slack webhook", "https://hooks.slack.com/services/T000/B000/XXXX", "<CREDENTIAL>"},
		{"stripe", "sk_live_" + repeat("a", 24), "<CREDENTIAL>"},
		{"google", "AIza" + repeat("b", 35), "<CREDENTIAL>"},
		{"openai", "sk-proj-" + repeat("c", 30), "<CREDENTIAL>"},
		{"anthropic", "sk-ant-" + repeat("d", 30), "<CREDENTIAL>"},
		{"npm", "npm_" + repeat("e", 36), "<CREDENTIAL>"},
		{"sendgrid", "SG." + repeat("f", 22) + "." + repeat("g", 43), "<CREDENTIAL>"},
		{"jwt", "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJqc21pdGgifQ.c2lnbmF0dXJlMTIz", "<CREDENTIAL>"},
		{"private key", "key:\n-----BEGIN RSA PRIVATE KEY-----\nMIIE\nabc\n-----END RSA PRIVATE KEY-----\ndone", "key:\n<CREDENTIAL>\ndone"},
		{"certificate", "-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----", "<CREDENTIAL>"},
		{"ssh public key", "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAA user@host", "<CREDENTIAL> user@host"},
		{"bearer header", "Authorization: Bearer abc.def/ghi=", "Authorization: Bearer <CREDENTIAL>"},
		{"bearer jwt", "Bearer eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJqc21pdGgifQ.c2lnbmF0dXJlMTIz", "Bearer <CREDENTIAL>"},
		{"bearer prose", "send bearer tokens.", "send bearer tokens."},
		{"password assignment", "password=hunter2 next", "password=<CREDENTIAL> next"},
		{"api key colon", "api_key: 'x9y8z7'", "api_key: '<CREDENTIAL>'"},
		{"json style", `{"access_token": "abc123xyz"}`, `{"access_token": "<CREDENTIAL>"}`},
		{"env style", "DB_PASSWORD=s3cr3t!", "DB_PASSWORD=<CREDENTIAL>"},
		{"prose value", "password: required", "password: required"},
		{"code value", "token = getToken()", "token = getToken()"},
		{"path value", "PWD=/home/jsmith/app", "PWD=/home/<USER>/app"},
		{"variable value", "secret=${SECRET} token=$TOKEN", "secret=${SECRET} token=$TOKEN"},
		{"empty quoted", `password=""`, `password=""`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := masked(tc.in)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestTextIPAddresses(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		// Addresses that are counted.
		{"private", "db at 10.0.0.12 is open", "db at <IP> is open"},
		{"public", "C2 server 8.8.4.4", "C2 server <IP>"},
		{"repeated address", "10.0.0.1 then 10.0.0.2 then 10.0.0.1", "<IP> then <IP> then <IP>"},
		{"user at host", "ssh root@10.20.30.40 then", "ssh root@<IP> then"},
		{"port", "10.1.2.3:8080", "<IP>:8080"},
		{"cidr", "10.1.2.9/24", "<IP>/24"},
		{"list", "10.1.2.6,10.1.2.7", "<IP>,<IP>"},
		{"range", "swept 10.20.30.1-10.20.30.254", "swept <IP>-<IP>"},
		{"underscore", "host_10.20.30.40 answered", "host_<IP> answered"},
		{"underscores around", "scan_10.20.30.40_full.xml", "scan_<IP>_full.xml"},
		{"single word host", "reached node-10.20.30.40", "reached node-<IP>"},
		{"dotted host", "via db.corp-web-10.20.30.40", "via db.corp-web-<IP>"},
		{"in url", "http://192.168.1.10:8080/admin", "http://<IP>:8080/admin"},
		{"after purl", "pkg:npm/a@1.0.0 then ssh root@10.20.30.40", "pkg:npm/a@1.0.0 then ssh root@<IP>"},
		{"ipv6", "listens on 2a00:1450:4001:82a::200e", "listens on <IP>"},
		{"ipv6 unique local", "fd12:3456:789a::1", "<IP>"},
		{"ipv6 same address", "2a00:1450::1 and 2a00:1450:0:0::1", "<IP> and <IP>"},
		// Addresses that identify nobody.
		{"loopback", "curl 127.0.0.1 and ::1", "curl 127.0.0.1 and ::1"},
		{"metadata", "SSRF to 169.254.169.254", "SSRF to 169.254.169.254"},
		{"unspecified", "bind 0.0.0.0", "bind 0.0.0.0"},
		{"broadcast", "255.255.255.255", "255.255.255.255"},
		{"documentation", "192.0.2.1 198.51.100.7 203.0.113.9 2001:db8::1", "192.0.2.1 198.51.100.7 203.0.113.9 2001:db8::1"},
		{"ipv6 link-local", "fe80::1", "fe80::1"},
		// Dotted numbers that are not addresses.
		{"hyphenated package", "daon-http-digest-auth-5.0.1.0 is affected", "daon-http-digest-auth-5.0.1.0 is affected"},
		{"jar", "jackson-databind-2.13.4.2.jar is bundled", "jackson-databind-2.13.4.2.jar is bundled"},
		{"underscore package", "python_dateutil_2.8.2.1 is pinned", "python_dateutil_2.8.2.1 is pinned"},
		{"purl version", "pkg:maven/com.x/daon-http-digest-auth@5.0.1.0 is affected", "pkg:maven/com.x/daon-http-digest-auth@5.0.1.0 is affected"},
		{"purl version repeated", "pkg:npm/a@5.0.1.0 fixed after 5.0.1.0", "pkg:npm/a@5.0.1.0 fixed after 5.0.1.0"},
		{"v prefix", "v1.2.3.4", "v1.2.3.4"},
		{"version word", "version 10.0.0.1 and release 2.1.0.4", "version 10.0.0.1 and release 2.1.0.4"},
		{"section", "RFC 4880 §5.2.3.10 and section 5.4.3.1", "RFC 4880 §5.2.3.10 and section 5.4.3.1"},
		{"oid", "OID 1.3.6.1.4.1.311", "OID 1.3.6.1.4.1.311"},
		{"too long", "1234.1.1.1", "1234.1.1.1"},
		{"invalid octet", "300.1.1.1", "300.1.1.1"},
		{"leading zero", "010.001.001.001", "010.001.001.001"},
		{"cpp scope", "std::vector and Foo::Bar", "std::vector and Foo::Bar"},
		{"mac", "aa:bb:cc:dd:ee:ff", "aa:bb:cc:dd:ee:ff"},
		{"time", "at 12:34:56", "at 12:34:56"},
		{"bare double colon", "a :: b", "a :: b"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := masked(tc.in)
			assert.Equal(t, tc.want, got)
		})
	}
}

// masked returns the working copy with each finding replaced by a marker, which
// shows exactly which spans the rules found.
func masked(s string) string {
	return newScanner().mask(s)
}

func TestTextStats(t *testing.T) {
	stats := Text("a@corp.com b@corp.com /home/jsmith AKIAIOSFODNN7EXAMPLE 10.0.0.5")
	assert.Equal(t, Stats{CategoryEmail: 2, CategoryPathUser: 1, CategoryCredential: 1, CategoryIP: 1}, stats)
	assert.Equal(t, 5, stats.Total())
}

func TestTextCountsEveryOccurrence(t *testing.T) {
	assert.Equal(t, Stats{CategoryEmail: 3}, Text("a@corp.com a@corp.com A@CORP.COM"))
}

func TestTextCountsAValueOnce(t *testing.T) {
	// The IP inside the password value is part of the credential, not a second finding.
	assert.Equal(t, Stats{CategoryCredential: 1}, Text("database_url=postgres://app:pw@10.0.0.5/db"))
}

func TestJSONCountsFindings(t *testing.T) {
	body := `{
  "name": "Batch from jsmith@corp.com",
  "reporter": {"email": "jsmith@corp.com", "notes": ["see /home/jsmith/poc"]},
  "vulnerabilities": [
    {
      "vulnerability_id": "CVE-2024-1",
      "packageurl": "pkg:maven/org.x/y@1.0.Final",
      "component_version": "10.2.3.4",
      "title": "Leak",
      "description": "Token ghp_` + repeat("a", 36) + ` found in C:\\Users\\jsmith\\app on 10.0.0.5",
      "cvss_score": 9.8,
      "exploit_tested": true,
      "reproducer_filename": "jsmith@corp.com-poc.sh"
    }
  ]
}`
	stats, err := JSON([]byte(body))
	require.NoError(t, err)
	assert.Equal(t, Stats{CategoryEmail: 2, CategoryPathUser: 2, CategoryCredential: 1, CategoryIP: 1}, stats)
}

func TestJSONNoFindings(t *testing.T) {
	stats, err := JSON([]byte(`{"name":"batch","vulnerabilities":[{"cvss_score":7.50,"title":"t"}]}`))
	require.NoError(t, err)
	assert.Zero(t, stats.Total())
}

func TestJSONSkipsComponentFields(t *testing.T) {
	stats, err := JSON([]byte(`{"component_version":"10.2.3.4","component_name":"10.2.3.4","description":"10.2.3.4"}`))
	require.NoError(t, err)
	assert.Equal(t, Stats{CategoryIP: 1}, stats)
}

func TestJSONDecodesEscapes(t *testing.T) {
	stats, err := JSON([]byte(`{"d":"jsmith\u0040corp.com \"quoted\""}`))
	require.NoError(t, err)
	assert.Equal(t, Stats{CategoryEmail: 1}, stats)
}

func TestJSONIgnoresKeys(t *testing.T) {
	stats, err := JSON([]byte(`{"jsmith@corp.com":"no findings here"}`))
	require.NoError(t, err)
	assert.Zero(t, stats.Total())
}

func TestJSONSkippedKeyOnlyAffectsItsValue(t *testing.T) {
	stats, err := JSON([]byte(`{"packageurl":"a@corp.com","next":"a@corp.com","list":["a@corp.com"]}`))
	require.NoError(t, err)
	assert.Equal(t, Stats{CategoryEmail: 2}, stats)
}

func TestJSONInvalid(t *testing.T) {
	for _, body := range []string{`not json`, `{"a":`, `{"a" "b"}`, `[1,]`} {
		_, err := JSON([]byte(body))
		assert.Error(t, err, body)
	}
}

func repeat(s string, n int) string {
	out := ""
	for range n {
		out += s
	}
	return out
}
