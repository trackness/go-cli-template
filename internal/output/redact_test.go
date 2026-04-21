package output_test

import (
	"strings"
	"testing"

	"github.com/example/go-cli-template/internal/output"
)

func TestIsSensitive(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want bool
	}{
		{"literal token", "token", true},
		{"literal password", "password", true},
		{"literal secret", "secret", true},
		{"suffix -token", "api-token", true},
		{"suffix -secret", "client-secret", true},
		{"suffix -password", "admin-password", true},
		{"suffix -key", "encryption-key", true},
		{"upper-case literal", "TOKEN", true},
		{"upper-case suffix", "API-TOKEN", true},
		{"mixed case", "Api-Token", true},
		{"non-sensitive", "endpoint", false},
		{"non-sensitive username", "username", false},
		{"non-sensitive log-level", "log-level", false},
		{"non-sensitive timeout", "timeout", false},
		{"empty string", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := output.IsSensitive(tt.key); got != tt.want {
				t.Errorf("IsSensitive(%q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}

func TestRedactValue_SensitiveKey_ReturnsRedacted(t *testing.T) {
	got := output.RedactValue("api-token", "secret-value-xxx")
	if got != output.Redacted {
		t.Errorf("RedactValue(api-token, ...) = %v, want %q", got, output.Redacted)
	}
}

func TestRedactValue_NonSensitiveKey_PassesThrough(t *testing.T) {
	got := output.RedactValue("endpoint", "https://example.com")
	if got != "https://example.com" {
		t.Errorf("RedactValue(endpoint, ...) = %v, want passthrough", got)
	}
}

func TestIsSensitiveHeader(t *testing.T) {
	tests := []struct {
		name string
		h    string
		want bool
	}{
		{"Authorization", "Authorization", true},
		{"lower authorization", "authorization", true},
		{"Cookie", "Cookie", true},
		{"Proxy-Authorization", "Proxy-Authorization", true},
		{"X-API-Token", "X-API-Token", true},
		{"X-Foo-Secret", "X-Foo-Secret", true},
		{"X-Signing-Key", "X-Signing-Key", true},
		{"Content-Type", "Content-Type", false},
		{"User-Agent", "User-Agent", false},
		{"Accept", "Accept", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := output.IsSensitiveHeader(tt.h); got != tt.want {
				t.Errorf("IsSensitiveHeader(%q) = %v, want %v", tt.h, got, tt.want)
			}
		})
	}
}

func TestRedactCurl_AuthorizationHeaderRedacted(t *testing.T) {
	in := `curl -X GET 'http://api.example.com' -H 'Authorization: Bearer abc123secret' -H 'User-Agent: test/1.0'`
	got := output.RedactCurl(in)
	if strings.Contains(got, "abc123secret") {
		t.Errorf("secret token leaked: %q", got)
	}
	if !strings.Contains(got, output.Redacted) {
		t.Errorf("redacted placeholder missing: %q", got)
	}
	if !strings.Contains(got, "User-Agent: test/1.0") {
		t.Errorf("non-sensitive header lost: %q", got)
	}
}

func TestRedactCurl_DoubleQuotedHeader(t *testing.T) {
	in := `curl -X POST "http://x" -H "X-API-Token: xyz123"`
	got := output.RedactCurl(in)
	if strings.Contains(got, "xyz123") {
		t.Errorf("secret leaked: %q", got)
	}
}

func TestRedactCurl_NonSensitiveHeaders_PassThrough(t *testing.T) {
	in := `curl -X GET 'http://x' -H 'Content-Type: application/json' -H 'Accept: application/json'`
	got := output.RedactCurl(in)
	if got != in {
		t.Errorf("non-sensitive curl mutated:\n  before: %q\n  after:  %q", in, got)
	}
}

func TestRedactCurl_ShellEscapedQuoteInValue(t *testing.T) {
	// A Bearer token containing a literal single-quote is legal and
	// gets shell-escaped by resty as ' closed, " open, ' char, " close,
	// ' reopen. The regex-based approach stopped at the first quote
	// and leaked the tail; the split-based approach redacts the whole
	// value regardless of embedded escape sequences.
	in := `curl -X GET 'http://x' -H 'Authorization: Bearer pre'"'"'post'`
	got := output.RedactCurl(in)
	if strings.Contains(got, "pre") || strings.Contains(got, "post") {
		t.Errorf("partial leak — expected both halves of shell-escaped secret redacted: %q", got)
	}
	if !strings.Contains(got, output.Redacted) {
		t.Errorf("redacted placeholder missing: %q", got)
	}
}

func TestRedactCurl_RestyDumpHeadersBlock(t *testing.T) {
	// Resty's EnableGenerateCurlOnDebug emits the curl line PLUS a
	// REQUEST dump whose HEADERS block reprints every header tab-
	// indented. The curl redaction alone misses this second leak.
	in := "~~~ REQUEST(CURL) ~~~\n" +
		"\tcurl -X GET -H 'Authorization: Bearer SECRET_TOKEN' http://x\n" +
		"~~~ REQUEST ~~~\n" +
		"GET  /  HTTP/1.1\n" +
		"HEADERS:\n" +
		"\tAuthorization: Bearer SECRET_TOKEN\n" +
		"\tCookie: session=ABC123\n" +
		"\tUser-Agent: test/1.0\n"
	got := output.RedactCurl(in)
	if strings.Contains(got, "SECRET_TOKEN") {
		t.Errorf("secret leaked through dump block: %q", got)
	}
	if strings.Contains(got, "ABC123") {
		t.Errorf("cookie value leaked through dump block: %q", got)
	}
	if !strings.Contains(got, "User-Agent: test/1.0") {
		t.Errorf("non-sensitive dump header lost: %q", got)
	}
}

// TestSensitiveKeys_Extendable ensures descendants can append their own
// sensitive keys. Uses t.Cleanup to restore the package-level state so
// this test doesn't leak into others.
func TestSensitiveKeys_Extendable(t *testing.T) {
	orig := output.SensitiveKeys
	t.Cleanup(func() { output.SensitiveKeys = orig })

	output.SensitiveKeys = append(output.SensitiveKeys, "session-id")
	if !output.IsSensitive("session-id") {
		t.Errorf("after appending %q, IsSensitive returned false", "session-id")
	}
}
