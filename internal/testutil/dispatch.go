// Package testutil holds small shared test helpers. Kept minimal on
// purpose: per CLAUDE.md "HTTP response mocking", tests prefer real
// httptest.Server handlers wired by hand; testutil exists so that
// multi-endpoint handlers stay readable, not to grow into a mocking
// framework.
package testutil

import (
	"fmt"
	"net/http"
	"sort"
	"testing"
)

// Dispatch returns an http.HandlerFunc that routes requests to the
// per-(method, path) handler in routes. Keys are "<METHOD> <PATH>"
// (e.g. "GET /v1/items"). An unmatched request fails the test and
// returns 404 — unmatched traffic in a test fixture is a bug, not a
// silent-pass.
//
// Example:
//
//	srv := httptest.NewServer(testutil.Dispatch(t, map[string]http.HandlerFunc{
//	    "GET /health": func(w http.ResponseWriter, _ *http.Request) {
//	        w.WriteHeader(http.StatusOK)
//	    },
//	    "POST /items": createItemHandler(t),
//	}))
func Dispatch(t testing.TB, routes map[string]http.HandlerFunc) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		key := fmt.Sprintf("%s %s", r.Method, r.URL.Path)
		h, ok := routes[key]
		if !ok {
			t.Errorf("unexpected request: %s (known routes: %v)", key, sortedKeys(routes))
			http.NotFound(w, r)
			return
		}
		h(w, r)
	}
}

func sortedKeys(m map[string]http.HandlerFunc) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
