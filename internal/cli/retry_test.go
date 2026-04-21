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

// Retry timing tests that would otherwise sleep for real wall-clock
// seconds are implemented in retry_sync_test.go using testing/synctest
// — TestSynctest_RetryAfter_* there exercises the same retry contract
// in zero real time. newTestClient no longer exists; tests that want
// a resty client should either build one inline via httptest
// (non-timing tests) or use newSyncTestClient inside a synctest
// bubble (timing tests).

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

func TestRetryCondition_POSTNotRetried(t *testing.T) {
	// retryCondition restricts retries to GET/HEAD. A POST that
	// returns 503 must NOT be retried — the skill decides, not us.
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)

	// Build a resty client wired identically to buildDeps' production
	// config so the test exercises the same retry pipeline.
	client := resty.New().
		SetBaseURL(srv.URL).
		SetRetryCount(2).
		SetRetryWaitTime(retryBaseWait).
		SetRetryMaxWaitTime(retryAfterCap).
		SetRetryAfter(retryAfter).
		OnAfterResponse(enforceRetryAfterCap).
		AddRetryCondition(retryCondition)
	_, _ = client.R().Post("/")
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
// through deps.Resty and surfaces the resty error (if any) as the
// command's RunE return. Used to exercise full-pipeline paths
// (PersistentPreRunE → buildDeps → backfill → resty request) in
// situations where calling buildDeps directly would skip
// command-tree concerns like env backfill. Returning the error means
// tests can assert on transport failures via writeErrorAndExit's
// structured envelope rather than only via handler side-effects.
func newFetchCmd(baseURL string) *cobra.Command {
	return &cobra.Command{
		Use: "test-fetch",
		RunE: func(c *cobra.Command, _ []string) error {
			deps := depsFromContext(c.Context())
			deps.Resty.SetBaseURL(baseURL)
			_, err := deps.Resty.R().Get("/")
			return err
		},
	}
}

func TestRetryAfterCap_FiresUnderNoRetry(t *testing.T) {
	// F5: the 10s Retry-After cap must fire regardless of --no-retry.
	// Previously the abort lived inside the RetryAfter callback which
	// resty skips when SetRetryCount is 0, so a hostile server could
	// hand a --no-retry invocation a raw 429 with Retry-After: 99999.
	// The cap moves to OnAfterResponse which always fires.
	sterilizeEnv(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "15")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	t.Cleanup(srv.Close)

	flags := &Flags{Output: "json", LogLevel: "info", Timeout: 5 * time.Second, NoRetry: true}
	deps, err := buildDeps(BuildInfo{Version: "test-v0.0.0"}, flags, newBuildDepsPflagSet())
	if err != nil {
		t.Fatalf("buildDeps: %v", err)
	}
	deps.Resty.SetBaseURL(srv.URL)

	_, err = deps.Resty.R().Get("/")
	if err == nil {
		t.Fatal("err = nil, want RETRY_AFTER_TOO_LONG even with --no-retry")
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

func TestRunCmdTree_NoGoroutineLeak_PersistentPreRunError(t *testing.T) {
	// Belt-and-braces for the fourth exit path: PersistentPreRunE
	// itself fails (here via an invalid --log-level from env) BEFORE
	// the context.WithTimeout wrap is installed. No timer was ever
	// created, so the cancelTimeout guard at runCmdTree's defer
	// correctly no-ops. goleak locks the invariant — any future
	// refactor that installs the timer before the validation error
	// path would fail here rather than silently leak.
	defer goleak.VerifyNone(t)

	sterilizeEnv(t)
	t.Setenv("GO_CLI_TEMPLATE_LOG_LEVEL", "not-a-level")

	var stdout, stderr bytes.Buffer
	cmd := NewRoot(BuildInfo{Version: "test-v0.0.0"}, &stdout, &stderr)
	code := runCmdTree(context.Background(), cmd, []string{"version"})
	if code != output.ExitUserError {
		t.Errorf("exit code = %d, want %d (stderr=%q)", code, output.ExitUserError, stderr.String())
	}
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
