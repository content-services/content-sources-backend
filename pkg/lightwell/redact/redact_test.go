package redact

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTextEmails(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"plain", "contact jsmith@corp.com now", "contact <EMAIL_1> now"},
		{"example domain", "attacker@example.com", "<EMAIL_1>"},
		{"same address same number", "a@x.com b@y.org A@X.COM", "<EMAIL_1> <EMAIL_2> <EMAIL_1>"},
		{"unicode local part", "ivan.petrov@corp.ru and josé@corp.es", "<EMAIL_1> and <EMAIL_2>"},
		{"full-width at sign", "tanaka＠corp.jp", "<EMAIL_1>"},
		{"mailto", "mailto:sec@corp.com", "mailto:<EMAIL_1>"},
		{"digit-label provider", "user@163.com", "<EMAIL_1>"},
		{"maven version", "spring-core@5.3.RELEASE and name@1.0.Final", "spring-core@5.3.RELEASE and name@1.0.Final"},
		{"npm version", "lodash@4.17.21", "lodash@4.17.21"},
		{"scp remote", "git clone git@github.com:org/repo.git", "git clone git@github.com:org/repo.git"},
		{"sentence colon", "write to a@corp.com: thanks", "write to <EMAIL_1>: thanks"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := Text(tc.in)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestTextPathUsers(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"home", "/home/jsmith/app/config.yml", "/home/<USER_1>/app/config.yml"},
		{"macos", "/Users/jsmith/Library/Logs", "/Users/<USER_1>/Library/Logs"},
		{"windows", `C:\Users\jsmith\AppData\Local`, `C:\Users\<USER_1>\AppData\Local`},
		{"windows escaped", `C:\\Users\\jsmith\\AppData`, `C:\\Users\\<USER_1>\\AppData`},
		{"windows forward slash", "c:/users/jsmith/x", "c:/users/<USER_1>/x"},
		{"bare home", "files live in /home/jsmith.", "files live in /home/<USER_1>."},
		{"same user same number", "/home/jsmith/a /Users/kbrown/b /home/jsmith/c", "/home/<USER_1>/a /Users/<USER_2>/b /home/<USER_1>/c"},
		{"passwd line", "jsmith:x:1000:1000::/home/jsmith:/bin/bash", "jsmith:x:1000:1000::/home/<USER_1>:/bin/bash"},
		{"PATH list", "PATH=/home/kbrown/bin:/usr/bin", "PATH=/home/<USER_1>/bin:/usr/bin"},
		{"markdown link", "[notes](/home/kbrown)", "[notes](/home/<USER_1>)"},
		{"csv row", "web01,/home/jli,/opt/app", "web01,/home/<USER_1>,/opt/app"},
		{"dotted name", "/home/dan.oneil/.profile", "/home/<USER_1>/.profile"},
		{"file url", "file:///home/jsmith/x", "file:///home/<USER_1>/x"},
		{"system paths untouched", "/etc/passwd /opt/app/bin /usr/lib64/libssl.so node_modules/x/y.js", "/etc/passwd /opt/app/bin /usr/lib64/libssl.so node_modules/x/y.js"},
		{"placeholders untouched", "/home/$USER/x /home/${USER}/x C:\\Users\\%USERNAME%\\x /home/<user>/x /home/{user}/x", "/home/$USER/x /home/${USER}/x C:\\Users\\%USERNAME%\\x /home/<user>/x /home/{user}/x"},
		{"shared dirs untouched", "/Users/Shared/x C:\\Users\\Public\\x", "/Users/Shared/x C:\\Users\\Public\\x"},
		{"remote url untouched", "https://github.com/org/repo/blob/main/home/jsmith/x", "https://github.com/org/repo/blob/main/home/jsmith/x"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := Text(tc.in)
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
		{"path value", "PWD=/home/jsmith/app", "PWD=/home/<USER_1>/app"},
		{"variable value", "secret=${SECRET} token=$TOKEN", "secret=${SECRET} token=$TOKEN"},
		{"empty quoted", `password=""`, `password=""`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := Text(tc.in)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestTextStats(t *testing.T) {
	_, stats := Text("a@corp.com b@corp.com /home/jsmith AKIAIOSFODNN7EXAMPLE")
	assert.Equal(t, Stats{CategoryEmail: 2, CategoryPathUser: 1, CategoryCredential: 1}, stats)
	assert.Equal(t, 4, stats.Total())
}

func TestJSONUnchangedIsIdentical(t *testing.T) {
	body := []byte("{\n  \"name\": \"batch\",\n  \"vulnerabilities\": [ {\"cvss_score\": 7.50, \"title\": \"t\"} ]\n}\n")
	got, stats, err := JSON(body)
	require.NoError(t, err)
	assert.Equal(t, string(body), string(got))
	assert.Zero(t, stats.Total())
}

func TestJSONRedactsValuesAndKeepsLayout(t *testing.T) {
	body := `{
  "name": "Batch from jsmith@corp.com",
  "reporter": {"email": "jsmith@corp.com", "notes": ["see /home/jsmith/poc"]},
  "vulnerabilities": [
    {
      "vulnerability_id": "CVE-2024-1",
      "packageurl": "pkg:maven/org.x/y@1.0.Final",
      "title": "Leak",
      "description": "Token ghp_` + repeat("a", 36) + ` found in C:\\Users\\jsmith\\app <b>",
      "cvss_score": 9.8,
      "exploit_tested": true,
      "reproducer_filename": "jsmith@corp.com-poc.sh"
    }
  ]
}`
	got, stats, err := JSON([]byte(body))
	require.NoError(t, err)
	want := `{
  "name": "Batch from <EMAIL_1>",
  "reporter": {"email": "<EMAIL_1>", "notes": ["see /home/<USER_1>/poc"]},
  "vulnerabilities": [
    {
      "vulnerability_id": "CVE-2024-1",
      "packageurl": "pkg:maven/org.x/y@1.0.Final",
      "title": "Leak",
      "description": "Token <CREDENTIAL> found in C:\\Users\\<USER_1>\\app <b>",
      "cvss_score": 9.8,
      "exploit_tested": true,
      "reproducer_filename": "jsmith@corp.com-poc.sh"
    }
  ]
}`
	assert.Equal(t, want, string(got))
	assert.True(t, json.Valid(got))
	assert.Equal(t, Stats{CategoryEmail: 2, CategoryPathUser: 2, CategoryCredential: 1}, stats)
}

func TestJSONDecodesEscapes(t *testing.T) {
	got, _, err := JSON([]byte(`{"d":"jsmith\u0040corp.com \"quoted\""}`))
	require.NoError(t, err)
	assert.Equal(t, `{"d":"<EMAIL_1> \"quoted\""}`, string(got))
}

func TestJSONKeepsKeys(t *testing.T) {
	got, _, err := JSON([]byte(`{"jsmith@corp.com":"jsmith@corp.com"}`))
	require.NoError(t, err)
	assert.Equal(t, `{"jsmith@corp.com":"<EMAIL_1>"}`, string(got))
}

func TestJSONSkippedKeyOnlyAffectsItsValue(t *testing.T) {
	got, _, err := JSON([]byte(`{"packageurl":"a@corp.com","next":"a@corp.com","list":["a@corp.com"]}`))
	require.NoError(t, err)
	assert.Equal(t, `{"packageurl":"a@corp.com","next":"<EMAIL_1>","list":["<EMAIL_1>"]}`, string(got))
}

func TestJSONInvalid(t *testing.T) {
	for _, body := range []string{`not json`, `{"a":`, `{"a" "b"}`, `[1,]`} {
		_, _, err := JSON([]byte(body))
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
