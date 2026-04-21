package cli

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/spf13/pflag"

	"github.com/example/go-cli-template/internal/output"
)

func TestParseRetryAfter_DeltaSeconds(t *testing.T) {
	d, err := parseRetryAfter("5")
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if d != 5*time.Second {
		t.Errorf("d = %v, want 5s", d)
	}
}

func TestParseRetryAfter_Zero(t *testing.T) {
	d, err := parseRetryAfter("0")
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if d != 0 {
		t.Errorf("d = %v, want 0", d)
	}
}

func TestParseRetryAfter_HTTPDate_InFuture(t *testing.T) {
	future := time.Now().Add(3 * time.Second).UTC().Format(http.TimeFormat)
	d, err := parseRetryAfter(future)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if d <= 0 || d > 4*time.Second {
		t.Errorf("d = %v, want ~3s", d)
	}
}

func TestParseRetryAfter_HTTPDate_InPast_ReturnsZero(t *testing.T) {
	past := time.Now().Add(-5 * time.Second).UTC().Format(http.TimeFormat)
	d, err := parseRetryAfter(past)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if d != 0 {
		t.Errorf("d = %v, want 0", d)
	}
}

func TestParseRetryAfter_Invalid(t *testing.T) {
	if _, err := parseRetryAfter("not-a-duration-or-date"); err == nil {
		t.Errorf("err = nil, want non-nil for garbage input")
	}
}

func TestJitteredBackoff_InitialAttemptIsBase(t *testing.T) {
	// attempt=1 → 100ms base, ±20% jitter → range [80ms, 120ms].
	for i := 0; i < 100; i++ {
		d := jitteredBackoff(1)
		if d < 80*time.Millisecond || d > 120*time.Millisecond {
			t.Errorf("attempt 1 d = %v, want in [80ms, 120ms]", d)
		}
	}
}

func TestJitteredBackoff_ExponentialProgression(t *testing.T) {
	// attempt=2 → 200ms base, ±20% → [160ms, 240ms].
	// attempt=3 → 400ms base, ±20% → [320ms, 480ms].
	for i := 0; i < 50; i++ {
		d2 := jitteredBackoff(2)
		if d2 < 160*time.Millisecond || d2 > 240*time.Millisecond {
			t.Errorf("attempt 2 d = %v, want in [160ms, 240ms]", d2)
		}
		d3 := jitteredBackoff(3)
		if d3 < 320*time.Millisecond || d3 > 480*time.Millisecond {
			t.Errorf("attempt 3 d = %v, want in [320ms, 480ms]", d3)
		}
	}
}

func TestJitteredBackoff_CapsAt5s(t *testing.T) {
	// Any attempt high enough that base * 2^(n-1) > 5s clamps to 5s
	// before applying the ±20% jitter, so the result is in [4s, 6s].
	for i := 0; i < 20; i++ {
		d := jitteredBackoff(10)
		if d < 4*time.Second || d > 6*time.Second {
			t.Errorf("attempt 10 d = %v, want in [4s, 6s] (cap 5s ±20%%)", d)
		}
	}
}

// newTestClient builds a minimal resty client hitting the given httptest
// server and wired with our retryAfter hook. Used by the Retry-After
// tests below.
func newTestClient(srv *httptest.Server) *resty.Client {
	return resty.New().
		SetBaseURL(srv.URL).
		SetRetryCount(2).
		SetRetryWaitTime(100 * time.Millisecond).
		SetRetryMaxWaitTime(5 * time.Second).
		SetRetryAfter(retryAfter).
		AddRetryCondition(retryCondition)
}

func TestRetryAfter_ServerValueWithinCap_Honoured(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 2 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	start := time.Now()
	resp, err := newTestClient(srv).R().Get("/")
	if err != nil {
		t.Fatalf("Get err = %v", err)
	}
	if resp.StatusCode() != http.StatusOK {
		t.Errorf("final status = %d, want 200", resp.StatusCode())
	}
	if calls != 2 {
		t.Errorf("handler calls = %d, want 2 (one retry)", calls)
	}
	// Honoured Retry-After=1 should have produced ≥ ~1s wait, not the
	// computed backoff default (~100ms ±20%).
	if elapsed := time.Since(start); elapsed < 800*time.Millisecond {
		t.Errorf("elapsed = %v, want ≥ ~1s (Retry-After not honoured)", elapsed)
	}
}

// newBuildDepsPflagSet mirrors the persistent flags registered on the
// root command so buildDeps can be called directly from tests without
// running the full cobra command tree.
func newBuildDepsPflagSet() *pflag.FlagSet {
	pfs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	pfs.String("output", "json", "")
	pfs.String("log-level", "info", "")
	pfs.Duration("timeout", 5*time.Second, "")
	pfs.String("config", "", "")
	pfs.Bool("yes", false, "")
	pfs.Bool("dry-run", false, "")
	pfs.Bool("no-retry", false, "")
	return pfs
}

func TestNoRetryFlag_DisablesRetry(t *testing.T) {
	sterilizeEnv(t)

	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)

	flags := &Flags{Output: "json", LogLevel: "info", Timeout: 5 * time.Second, NoRetry: true}
	deps, err := buildDeps(BuildInfo{Version: "test"}, flags, newBuildDepsPflagSet())
	if err != nil {
		t.Fatalf("buildDeps: %v", err)
	}
	deps.Resty.SetBaseURL(srv.URL)
	_, _ = deps.Resty.R().Get("/")

	if calls != 1 {
		t.Errorf("handler calls = %d, want 1 (--no-retry should disable retries)", calls)
	}
}

func TestDefaultRetry_RetriesTransientFailure(t *testing.T) {
	// Counter-test for the above: without --no-retry, 503 triggers the
	// default 2 retries (3 total attempts).
	sterilizeEnv(t)

	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)

	flags := &Flags{Output: "json", LogLevel: "info", Timeout: 5 * time.Second}
	deps, err := buildDeps(BuildInfo{Version: "test"}, flags, newBuildDepsPflagSet())
	if err != nil {
		t.Fatalf("buildDeps: %v", err)
	}
	deps.Resty.SetBaseURL(srv.URL)
	_, _ = deps.Resty.R().Get("/")

	if calls != 3 {
		t.Errorf("handler calls = %d, want 3 (1 initial + 2 default retries)", calls)
	}
}

func TestRetryAfter_ServerValueAboveCap_FailsFast(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", strconv.Itoa(int((15 * time.Second).Seconds())))
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	t.Cleanup(srv.Close)

	start := time.Now()
	_, err := newTestClient(srv).R().Get("/")
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("err = nil, want RETRY_AFTER_TOO_LONG")
	}
	var oe *output.Error
	if !errors.As(err, &oe) {
		t.Fatalf("err not *output.Error: %v", err)
	}
	if oe.Code != output.ErrCodeRetryAfterTooLong {
		t.Errorf("code = %q, want %q", oe.Code, output.ErrCodeRetryAfterTooLong)
	}
	if oe.ExitCode != output.ExitTargetError {
		t.Errorf("exit code = %d, want %d", oe.ExitCode, output.ExitTargetError)
	}
	// Fail-fast: should not have slept anywhere near 15s.
	if elapsed > 2*time.Second {
		t.Errorf("elapsed = %v, want fail-fast (< 2s) not wait for Retry-After", elapsed)
	}
}
