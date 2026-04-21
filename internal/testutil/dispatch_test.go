package testutil_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/example/go-cli-template/internal/testutil"
)

func TestDispatch_RoutesKnownKey(t *testing.T) {
	srv := httptest.NewServer(testutil.Dispatch(t, map[string]http.HandlerFunc{
		"GET /hello": func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("world"))
		},
	}))
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/hello")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "world" {
		t.Errorf("body = %q, want %q", string(body), "world")
	}
}

func TestDispatch_UnknownKey_FailsTestAndReturns404(t *testing.T) {
	// Run as a subtest so the parent can observe the failure without
	// aborting itself. A real test using Dispatch wants unmatched
	// traffic to fail loudly — verify that it does.
	fake := &testing.T{}
	handler := testutil.Dispatch(fake, map[string]http.HandlerFunc{
		"GET /known": func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
	})

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/unknown")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
	if !fake.Failed() {
		t.Errorf("expected dispatch to fail the test on unmatched route, but t.Failed() = false")
	}
}
