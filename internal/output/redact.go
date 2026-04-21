package output

import (
	"regexp"
	"strings"
)

// Redacted is the placeholder value that replaces sensitive data at
// every rendering site (config view values, debug curl headers). Skills
// reading config view JSON see this literal rather than the secret.
const Redacted = "<redacted>"

// SensitiveKeys lists config-key names treated as secrets by literal
// equality. Comparison is case-insensitive. Descendants may append
// target-specific keys:
//
//	output.SensitiveKeys = append(output.SensitiveKeys, "session-id")
//
// Appending is the supported extension; reassigning the slice wholesale
// works but loses the default set.
var SensitiveKeys = []string{
	"token",
	"password",
	"secret",
}

// SensitiveSuffixes lists config-key suffixes treated as secrets.
// Matching is lowercase. Under the template's flat-dash key
// convention, keys like `api-token` match `-token`, `github-key`
// matches `-key`, etc. Descendants extend by appending.
var SensitiveSuffixes = []string{
	"-token",
	"-secret",
	"-password",
	"-key",
}

// SensitiveHeaders lists HTTP header names treated as secrets in
// rendered curl debug output, regardless of whether their suffix
// matches SensitiveSuffixes. Comparison is case-insensitive.
// Descendants extend by appending.
var SensitiveHeaders = []string{
	"authorization",
	"proxy-authorization",
	"cookie",
	"set-cookie",
}

// IsSensitive reports whether key names a secret per the default set
// (or per any descendant-appended entry). Comparison is lowercase.
func IsSensitive(key string) bool {
	k := strings.ToLower(key)
	for _, lit := range SensitiveKeys {
		if k == strings.ToLower(lit) {
			return true
		}
	}
	for _, suffix := range SensitiveSuffixes {
		if strings.HasSuffix(k, strings.ToLower(suffix)) {
			return true
		}
	}
	return false
}

// IsSensitiveHeader reports whether the HTTP header name should be
// redacted in curl debug output. Applies both the explicit header
// allowlist and the same suffix heuristic as IsSensitive.
func IsSensitiveHeader(name string) bool {
	n := strings.ToLower(name)
	for _, h := range SensitiveHeaders {
		if n == strings.ToLower(h) {
			return true
		}
	}
	for _, suffix := range SensitiveSuffixes {
		if strings.HasSuffix(n, strings.ToLower(suffix)) {
			return true
		}
	}
	return false
}

// RedactValue returns Redacted when key is sensitive; otherwise v is
// returned unchanged. Callers that produce skill-facing output must
// route every (key, value) pair through this function before
// serialising.
func RedactValue(key string, v any) any {
	if IsSensitive(key) {
		return Redacted
	}
	return v
}

// curlHeaderRE matches a single curl -H argument in either quoting
// style. Submatches: 1=prefix up to opening quote, 2=header name,
// 3=separator (: and any spaces), 4=header value, 5=closing quote.
var curlHeaderRE = regexp.MustCompile(`(-H\s*['"])([A-Za-z0-9_-]+)(:\s*)([^'"]*)(['"])`)

// RedactCurl rewrites a curl command string so that the values of
// sensitive HTTP headers (Authorization, Cookie, anything matching
// IsSensitiveHeader) become Redacted. Non-sensitive headers and the
// rest of the curl command are passed through unchanged. Used by the
// slog/resty logger adapter to scrub debug curl dumps before they
// reach stderr.
func RedactCurl(s string) string {
	return curlHeaderRE.ReplaceAllStringFunc(s, func(match string) string {
		parts := curlHeaderRE.FindStringSubmatch(match)
		if len(parts) < 6 {
			return match
		}
		if IsSensitiveHeader(parts[2]) {
			return parts[1] + parts[2] + parts[3] + Redacted + parts[5]
		}
		return match
	})
}
