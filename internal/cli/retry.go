package cli

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"

	"github.com/example/go-cli-template/internal/output"
)

// retryAfterCap is the upper bound on server-requested Retry-After
// values. Anything longer fails the request immediately with
// RETRY_AFTER_TOO_LONG rather than blocking the caller. Matches
// CLAUDE.md "HTTP retry".
const retryAfterCap = 10 * time.Second

// retryBaseWait is the initial backoff delay between retries.
const retryBaseWait = 100 * time.Millisecond

// retryMaxWait is the per-attempt cap on the computed exponential
// backoff (before ±20% jitter is applied).
const retryMaxWait = 5 * time.Second

// retryCondition enforces the contract-mandated retry policy: GET and
// HEAD only; status 429/502/503/504 or transport error. PUT, POST,
// DELETE, and PATCH are never auto-retried — the skill decides.
func retryCondition(resp *resty.Response, err error) bool {
	if resp == nil {
		return err != nil
	}
	req := resp.Request
	if req == nil {
		return false
	}
	if req.Method != http.MethodGet && req.Method != http.MethodHead {
		return false
	}
	switch resp.StatusCode() {
	case http.StatusTooManyRequests,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	}
	return false
}

// retryAfter is the resty RetryAfter callback. It honours the server's
// Retry-After header up to retryAfterCap; beyond the cap it returns a
// structured error that fails the request rather than blocks. When the
// server does not send Retry-After, falls back to an exponential
// backoff with ±20% jitter.
func retryAfter(_ *resty.Client, resp *resty.Response) (time.Duration, error) {
	if resp != nil {
		if raw := resp.Header().Get("Retry-After"); raw != "" {
			d, err := parseRetryAfter(raw)
			if err == nil {
				if d > retryAfterCap {
					return 0, &output.Error{
						Code:    output.ErrCodeRetryAfterTooLong,
						Message: fmt.Sprintf("server requested Retry-After %s (cap %s); aborting rather than blocking", d, retryAfterCap),
						Details: map[string]any{
							"retry_after": d.String(),
							"cap":         retryAfterCap.String(),
						},
						ExitCode: output.ExitTargetError,
					}
				}
				if d < 0 {
					d = 0
				}
				return d, nil
			}
			// Parse failure — fall through to computed backoff.
		}
	}
	attempt := 1
	if resp != nil && resp.Request != nil {
		attempt = resp.Request.Attempt
	}
	return jitteredBackoff(attempt), nil
}

// parseRetryAfter accepts both delta-seconds and HTTP-date forms per
// RFC 7231 §7.1.3. A past HTTP-date or negative delta-seconds returns
// 0 (retry immediately). Garbage input returns an error that carries
// both the Atoi and ParseTime failures so the caller can diagnose at
// debug level rather than treating the header as zero wait.
func parseRetryAfter(raw string) (time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, fmt.Errorf("empty Retry-After")
	}
	n, atoiErr := strconv.Atoi(raw)
	if atoiErr == nil {
		if n < 0 {
			return 0, nil
		}
		return time.Duration(n) * time.Second, nil
	}
	t, parseErr := http.ParseTime(raw)
	if parseErr == nil {
		d := time.Until(t)
		if d < 0 {
			return 0, nil
		}
		return d, nil
	}
	return 0, fmt.Errorf("invalid Retry-After %q: %w", raw, errors.Join(atoiErr, parseErr))
}

// jitteredBackoff returns the wait duration before the next retry.
// attempt is 1-indexed against resty's Request.Attempt counter:
// attempt=1 is the wait *after the first failed request* (i.e. before
// the first retry) and evaluates to the base 100ms ±20%. attempt=2
// doubles the base to 200ms ±20%, and so on, capped at retryMaxWait.
func jitteredBackoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	shift := attempt - 1
	if shift > 30 {
		shift = 30
	}
	base := retryBaseWait << shift
	if base > retryMaxWait {
		base = retryMaxWait
	}
	// spread is 20% of base in ns. For the const-defined base values
	// (100ms … 5s) spread is always > 0; no defensive guard needed.
	spread := int64(float64(base) * 0.2)
	// rand.Int64N(2*spread) produces [0, 2*spread); subtract spread to
	// centre the jitter on base, yielding [base-spread, base+spread).
	return base + time.Duration(rand.Int64N(2*spread)) - time.Duration(spread)
}
