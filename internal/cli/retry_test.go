package cli

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"go.uber.org/goleak"

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
// server and wired with the same retry-related settings buildDeps
// installs on the production client. Must mirror buildDeps so tests
// exercise the real resty interaction, not a divergent harness config.
func newTestClient(srv *httptest.Server) *resty.Client {
	return resty.New().
		SetBaseURL(srv.URL).
		SetRetryCount(2).
		SetRetryWaitTime(retryBaseWait).
		SetRetryMaxWaitTime(retryAfterCap).
		SetRetryAfter(retryAfter).
		AddRetryCondition(retryCondition)
}

func TestRetryAfter_ServerValueWithinCap_Honoured(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls < 2 {
			w.Header().Set("Retry-After", "2")
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
	// Retry-After: 2 should produce ~2s wait; default computed backoff
	// would be ~100-200ms. The 1500ms threshold cleanly discriminates
	// while leaving 500ms of CI jitter headroom.
	if elapsed := time.Since(start); elapsed < 1500*time.Millisecond {
		t.Errorf("elapsed = %v, want ≥ 1.5s (Retry-After not honoured)", elapsed)
	}
}

func TestRetryAfter_ServerValueBetweenComputedAndRetryAfterCap(t *testing.T) {
	// A Retry-After value in the [5s, 10s] window must be honoured,
	// not silently truncated to the computed-backoff cap (5s) by
	// resty's SetRetryMaxWaitTime. Exercises the H1 fix.
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls < 2 {
			w.Header().Set("Retry-After", "7")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	start := time.Now()
	_, err := newTestClient(srv).R().Get("/")
	if err != nil {
		t.Fatalf("Get err = %v", err)
	}
	// Expect ≥ 6.5s wait, well above the old 5s truncation ceiling.
	if elapsed := time.Since(start); elapsed < 6500*time.Millisecond {
		t.Errorf("elapsed = %v, want ≥ 6.5s (Retry-After 7 truncated to ≤ 5s — H1 regression)", elapsed)
	}
}

func TestRetryAfter_GarbageHeader_FallsThroughToBackoff(t *testing.T) {
	// An unparseable Retry-After value must not abort retries; fall
	// through to the computed backoff path.
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Retry-After", "hot-dog")
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)

	start := time.Now()
	_, _ = newTestClient(srv).R().Get("/")
	elapsed := time.Since(start)

	if calls != 3 {
		t.Errorf("calls = %d, want 3 (parse fallback must still retry)", calls)
	}
	// Elapsed should reflect the computed-backoff path (~100 + ~200 ≈
	// 300ms ±jitter), not a honoured Retry-After.
	if elapsed > 2*time.Second {
		t.Errorf("elapsed = %v, want computed-backoff (<2s); garbage header shouldn't have honoured anything", elapsed)
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "15")
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
	// Details payload is contract surface; skills branch on the
	// stringified duration and cap, not prose.
	if oe.Details["retry_after"] != "15s" {
		t.Errorf("details.retry_after = %v, want %q", oe.Details["retry_after"], "15s")
	}
	if oe.Details["cap"] != "10s" {
		t.Errorf("details.cap = %v, want %q", oe.Details["cap"], "10s")
	}
	// Fail-fast: should not have slept anywhere near 15s.
	if elapsed > 2*time.Second {
		t.Errorf("elapsed = %v, want fail-fast (< 2s) not wait for Retry-After", elapsed)
	}
}

func TestRetryCondition_POSTNotRetried(t *testing.T) {
	// retryCondition restricts retries to GET/HEAD. A POST that
	// returns 503 must NOT be retried — the skill decides, not us.
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)

	_, _ = newTestClient(srv).R().Post("/")
	if calls != 1 {
		t.Errorf("POST handler calls = %d, want 1 (retryCondition must not retry non-idempotent methods)", calls)
	}
}

func TestContextTimeout_BoundsRetryWindow(t *testing.T) {
	// PersistentPreRunE wraps the command context with
	// context.WithTimeout(ctx, f.Timeout). Resty's retry loop checks
	// ctx.Err() between attempts and stops when the deadline passes.
	// Simulate the PersistentPreRunE wrap directly here.
	sterilizeEnv(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)

	flags := &Flags{Output: "json", LogLevel: "info", Timeout: 100 * time.Millisecond}
	deps, err := buildDeps(BuildInfo{Version: "test"}, flags, newBuildDepsPflagSet())
	if err != nil {
		t.Fatalf("buildDeps: %v", err)
	}
	deps.Resty.SetBaseURL(srv.URL)

	ctx, cancel := context.WithTimeout(context.Background(), flags.Timeout)
	defer cancel()

	start := time.Now()
	_, _ = deps.Resty.R().SetContext(ctx).Get("/")
	elapsed := time.Since(start)

	// Without context timeout, three attempts + backoff ≈ 300ms+.
	// With 100ms timeout, the retry loop should bail within ~200ms.
	if elapsed > 300*time.Millisecond {
		t.Errorf("elapsed = %v, want context timeout to bound retries (≤ ~200ms)", elapsed)
	}
}

func TestInvalidTimeout_Errors(t *testing.T) {
	flags := &Flags{Output: "json", LogLevel: "info", Timeout: 0}
	_, err := buildDeps(BuildInfo{Version: "test"}, flags, newBuildDepsPflagSet())
	if err == nil {
		t.Fatal("buildDeps returned nil, want INVALID_FLAG for --timeout 0")
	}
	var oe *output.Error
	if !errors.As(err, &oe) {
		t.Fatalf("err not *output.Error: %v", err)
	}
	if oe.Code != output.ErrCodeInvalidFlag {
		t.Errorf("code = %q, want %q", oe.Code, output.ErrCodeInvalidFlag)
	}
}

// newFetchCmd returns a test-only leaf that makes a single GET
// through deps.Resty. Used to exercise full-pipeline paths
// (PersistentPreRunE → buildDeps → backfill → resty request) in
// situations where calling buildDeps directly would skip
// command-tree concerns like env backfill.
func newFetchCmd(baseURL string) *cobra.Command {
	return &cobra.Command{
		Use: "test-fetch",
		RunE: func(c *cobra.Command, _ []string) error {
			deps := depsFromContext(c.Context())
			deps.Resty.SetBaseURL(baseURL)
			_, _ = deps.Resty.R().Get("/")
			return nil
		},
	}
}

func TestNoRetry_EnvBackfill_DisablesRetry(t *testing.T) {
	// F3: --no-retry via env must travel flag>env>file>default
	// backfill onto Flags.NoRetry and reach the resty client. The
	// existing TestNoRetryFlag_DisablesRetry constructs Flags
	// directly, bypassing this path.
	sterilizeEnv(t)
	t.Setenv("GO_CLI_TEMPLATE_NO_RETRY", "true")

	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)

	var stdout, stderr bytes.Buffer
	cmd := NewRoot(BuildInfo{Version: "test-v0.0.0"}, &stdout, &stderr)
	cmd.AddCommand(newFetchCmd(srv.URL))

	code := runCmdTree(context.Background(), cmd, []string{"test-fetch"})
	if code != output.ExitSuccess {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, output.ExitSuccess, stderr.String())
	}
	if calls != 1 {
		t.Errorf("handler calls = %d, want 1 (--no-retry from env must reach resty)", calls)
	}
}

func TestRunCmdTree_NoGoroutineLeak_HappyPath(t *testing.T) {
	// F4: context.WithTimeout installed by PersistentPreRunE must be
	// released on the success path. goleak snapshots goroutines at
	// test start and fails if any created during the test remain
	// alive.
	defer goleak.VerifyNone(t)

	sterilizeEnv(t)
	var stdout, stderr bytes.Buffer
	cmd := NewRoot(BuildInfo{Version: "test-v0.0.0"}, &stdout, &stderr)
	code := runCmdTree(context.Background(), cmd, []string{"version"})
	if code != output.ExitSuccess {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, output.ExitSuccess, stderr.String())
	}
}

func TestRunCmdTree_NoGoroutineLeak_ErrorPath(t *testing.T) {
	// Error path — bare group triggers SUBCOMMAND_REQUIRED. Cancel
	// must still fire.
	defer goleak.VerifyNone(t)

	sterilizeEnv(t)
	var stdout, stderr bytes.Buffer
	cmd := NewRoot(BuildInfo{Version: "test-v0.0.0"}, &stdout, &stderr)
	_ = runCmdTree(context.Background(), cmd, []string{})
}

func TestRunCmdTree_NoGoroutineLeak_PanicPath(t *testing.T) {
	// Panic path — the recover deferred func must still fire cancel.
	defer goleak.VerifyNone(t)

	sterilizeEnv(t)
	var stdout, stderr bytes.Buffer
	cmd := NewRoot(BuildInfo{Version: "test-v0.0.0"}, &stdout, &stderr)
	cmd.AddCommand(&cobra.Command{
		Use: "test-panic",
		RunE: func(_ *cobra.Command, _ []string) error {
			panic("synthetic invariant")
		},
	})
	_ = runCmdTree(context.Background(), cmd, []string{"test-panic"})
}

func TestParseRetryAfter_NegativeDelta_ReturnsZero(t *testing.T) {
	d, err := parseRetryAfter("-5")
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if d != 0 {
		t.Errorf("d = %v, want 0", d)
	}
}
