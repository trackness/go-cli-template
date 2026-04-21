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
//
// set-cookie is a response-only header; it is listed here so that
// future response-logging paths (if added) cover it by default
// without each descendant remembering to add it.
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

// dumpHeaderRE matches the tab-indented "Name: value" lines resty
// emits inside the REQUEST dump block (distinct from the curl line).
// Without this pass the dump leaks secrets even when the curl line is
// scrubbed. Applied with (?m) so ^/$ anchor to line boundaries inside
// the multi-line Debugf payload.
var dumpHeaderRE = regexp.MustCompile(`(?m)^(\t)([A-Za-z0-9_-]+)(:\s*)(.*)$`)

// RedactCurl rewrites resty's debug output so that the values of
// sensitive HTTP headers (Authorization, Cookie, anything matching
// IsSensitiveHeader) become Redacted. Two passes:
//
//  1. The curl line itself — `-H '<name>: <value>'` or `-H "<name>: <value>"`.
//     Split-based (not regex) so shell-escaped quote sequences like
//     '"'"' inside the value don't truncate redaction at the first
//     embedded quote. Known limitation: a value containing the literal
//     " -H " byte sequence would fragment the split and leak;
//     pathological-input territory, documented in CLAUDE.md.
//  2. The REQUEST dump's HEADERS block — tab-indented `Name: value`
//     lines. Resty's EnableGenerateCurlOnDebug emits these alongside
//     the curl line; without this pass the token ships to stderr twice
//     (once curl-formatted, once dump-formatted).
//
// Non-sensitive headers and the rest of the dump pass through unchanged.
func RedactCurl(s string) string {
	s = redactCurlHFlags(s)
	s = redactDumpHeaders(s)
	return s
}

// redactCurlHFlags walks the curl command's -H arguments by splitting
// on " -H " rather than regex-matching. This handles shell-escaped
// quote sequences inside values (e.g. `'"'"'`) which defeat a
// quote-anchored regex.
func redactCurlHFlags(s string) string {
	parts := strings.Split(s, " -H ")
	if len(parts) < 2 {
		return s
	}
	for i := 1; i < len(parts); i++ {
		parts[i] = redactOneHArg(parts[i])
	}
	return strings.Join(parts, " -H ")
}

// redactOneHArg redacts one `'<name>: <value>'` argument (or
// "<name>: <value>") if name is sensitive. Scans from the END of the
// piece for the outer closing quote: a quote character followed by
// whitespace or end-of-string is the true close, regardless of how
// many shell-escape quote sequences precede it.
func redactOneHArg(piece string) string {
	if len(piece) < 2 {
		return piece
	}
	opener := piece[0]
	if opener != '\'' && opener != '"' {
		return piece
	}
	rel := strings.IndexByte(piece[1:], ':')
	if rel < 1 {
		return piece
	}
	colonIdx := rel + 1
	name := piece[1:colonIdx]
	if !IsSensitiveHeader(name) {
		return piece
	}
	closeIdx := -1
	for i := len(piece) - 1; i > colonIdx; i-- {
		if piece[i] == opener && (i == len(piece)-1 || piece[i+1] == ' ' || piece[i+1] == '\t') {
			closeIdx = i
			break
		}
	}
	if closeIdx == -1 {
		return piece
	}
	return string(opener) + name + ": " + Redacted + string(opener) + piece[closeIdx+1:]
}

// redactDumpHeaders rewrites tab-indented "Name: value" lines in
// resty's REQUEST dump so that sensitive headers carry only the
// placeholder.
func redactDumpHeaders(s string) string {
	return dumpHeaderRE.ReplaceAllStringFunc(s, func(match string) string {
		parts := dumpHeaderRE.FindStringSubmatch(match)
		if len(parts) < 5 {
			return match
		}
		if IsSensitiveHeader(parts[2]) {
			return parts[1] + parts[2] + parts[3] + Redacted
		}
		return match
	})
}
