package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"testing"
	"testing/synctest"
	"time"

	"github.com/go-resty/resty/v2"

	"github.com/example/go-cli-template/internal/output"
)

// fakeResponse describes one canned response for fakeTripper.
type fakeResponse struct {
	status  int
	headers map[string]string
}

// fakeTripper is an http.RoundTripper that returns canned responses
// from a fixed queue. Used so synctest-based tests can exercise the
// retry loop without real network I/O — the synctest bubble advances
// its fake clock only when all goroutines are durably blocked, and
// real HTTP calls are not considered durably blocked.
type fakeTripper struct {
	responses []fakeResponse
	calls     int
}

func (f *fakeTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if f.calls >= len(f.responses) {
		return nil, fmt.Errorf("fakeTripper: no more responses (queue exhausted at call %d)", f.calls)
	}
	r := f.responses[f.calls]
	f.calls++
	resp := &http.Response{
		StatusCode: r.status,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(nil)),
		Request:    req,
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
	}
	for k, v := range r.headers {
		resp.Header.Set(k, v)
	}
	return resp, nil
}

// newSyncTestClient mirrors buildDeps' resty wiring but hangs off a
// fakeTripper so every request lands on a canned response in the
// synctest bubble. All retry behaviour (jitter, Retry-After, cap
// enforcement, condition) is exercised identically to production.
func newSyncTestClient(tripper *fakeTripper) *resty.Client {
	return resty.NewWithClient(&http.Client{Transport: tripper}).
		SetBaseURL("http://synctest.invalid").
		SetRetryCount(2).
		SetRetryWaitTime(retryBaseWait).
		SetRetryMaxWaitTime(retryAfterCap).
		SetRetryAfter(retryAfter).
		OnAfterResponse(enforceRetryAfterCap).
		AddRetryCondition(retryCondition)
}

func TestSynctest_RetryAfter_ServerValueWithinCap_Honoured(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		tripper := &fakeTripper{responses: []fakeResponse{
			{status: http.StatusTooManyRequests, headers: map[string]string{"Retry-After": "2"}},
			{status: http.StatusOK},
		}}

		start := time.Now()
		resp, err := newSyncTestClient(tripper).R().Get("/")
		if err != nil {
			t.Fatalf("Get err = %v", err)
		}
		if resp.StatusCode() != http.StatusOK {
			t.Errorf("final status = %d, want 200", resp.StatusCode())
		}
		if tripper.calls != 2 {
			t.Errorf("calls = %d, want 2 (one retry after the 429)", tripper.calls)
		}
		// Bubble clock advances to honour the 2s Retry-After without
		// real wall-clock wait.
		if elapsed := time.Since(start); elapsed < 2*time.Second {
			t.Errorf("elapsed = %v, want ≥ 2s (Retry-After not honoured)", elapsed)
		}
	})
}

func TestSynctest_RetryAfter_ServerValueBetweenComputedAndRetryAfterCap(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		tripper := &fakeTripper{responses: []fakeResponse{
			{status: http.StatusTooManyRequests, headers: map[string]string{"Retry-After": "7"}},
			{status: http.StatusOK},
		}}

		start := time.Now()
		_, err := newSyncTestClient(tripper).R().Get("/")
		if err != nil {
			t.Fatalf("Get err = %v", err)
		}
		// 7s > 5s computed-backoff cap but < 10s Retry-After cap. The
		// cap applied to resty.SetRetryMaxWaitTime must be 10s or
		// this would silently truncate.
		if elapsed := time.Since(start); elapsed < 7*time.Second {
			t.Errorf("elapsed = %v, want ≥ 7s (Retry-After truncated — H1 regression)", elapsed)
		}
	})
}

func TestSynctest_RetryAfter_ServerValueAboveCap_FailsFast(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		tripper := &fakeTripper{responses: []fakeResponse{
			{status: http.StatusTooManyRequests, headers: map[string]string{"Retry-After": "15"}},
		}}

		start := time.Now()
		_, err := newSyncTestClient(tripper).R().Get("/")
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
		// Fail-fast: cap-enforcement in OnAfterResponse means we
		// never enter a wait at all.
		if elapsed > 100*time.Millisecond {
			t.Errorf("elapsed = %v, want near-zero (fail-fast)", elapsed)
		}
		if tripper.calls != 1 {
			t.Errorf("calls = %d, want 1 (structured error must halt retry loop)", tripper.calls)
		}
	})
}

func TestSynctest_RetryAfter_GarbageHeader_FallsThroughToBackoff(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		tripper := &fakeTripper{responses: []fakeResponse{
			{status: http.StatusServiceUnavailable, headers: map[string]string{"Retry-After": "hot-dog"}},
			{status: http.StatusServiceUnavailable, headers: map[string]string{"Retry-After": "hot-dog"}},
			{status: http.StatusServiceUnavailable, headers: map[string]string{"Retry-After": "hot-dog"}},
		}}

		_, _ = newSyncTestClient(tripper).R().Get("/")
		if tripper.calls != 3 {
			t.Errorf("calls = %d, want 3 (garbage header must fall through to retry loop)", tripper.calls)
		}
	})
}
