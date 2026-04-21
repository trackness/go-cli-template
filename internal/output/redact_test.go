package output_test

import (
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
	if contains(got, "abc123secret") {
		t.Errorf("secret token leaked: %q", got)
	}
	if !contains(got, output.Redacted) {
		t.Errorf("redacted placeholder missing: %q", got)
	}
	// Non-sensitive header must remain intact.
	if !contains(got, "User-Agent: test/1.0") {
		t.Errorf("non-sensitive header lost: %q", got)
	}
}

func TestRedactCurl_DoubleQuotedHeader(t *testing.T) {
	// Some curl generators use double quotes; redaction must cope.
	in := `curl -X POST "http://x" -H "X-API-Token: xyz123"`
	got := output.RedactCurl(in)
	if contains(got, "xyz123") {
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

// contains is a small helper to keep assertions readable; avoids
// importing strings into test just for one substring check per case.
func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
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
