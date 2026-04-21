package testutil_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/example/go-cli-template/internal/testutil"
)

func TestAssertGolden_Match(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "match.txt")
	if err := os.WriteFile(path, []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	testutil.AssertGolden(t, path, []byte("hello\n"))
}

// probe is a minimal fake *testing.T suitable for observing a
// helper's Failed() state without tripping the parent test. Relies on
// the helper using Errorf (not Fatalf) — the latter's runtime.Goexit
// would kill the caller's goroutine. AssertGolden / AssertJSONGolden
// are Errorf-only for exactly this reason.
func probe() *testing.T { return &testing.T{} }

func TestAssertGolden_Mismatch_FailsTest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mismatch.txt")
	if err := os.WriteFile(path, []byte("expected\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	p := probe()
	testutil.AssertGolden(p, path, []byte("actual\n"))
	if !p.Failed() {
		t.Errorf("AssertGolden with mismatched content did not fail the probe")
	}
}

func TestAssertGolden_MissingFile_FailsTest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.txt")
	p := probe()
	testutil.AssertGolden(p, path, []byte("anything"))
	if !p.Failed() {
		t.Errorf("AssertGolden with missing golden file did not fail the probe")
	}
}

func TestAssertJSONGolden_IndentsThenCompares(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "obj.json")
	// Pre-seed with indented form so the compact `got` below should
	// match after AssertJSONGolden re-indents it.
	indented := "{\n  \"k\": \"v\"\n}\n"
	if err := os.WriteFile(path, []byte(indented), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	testutil.AssertJSONGolden(t, path, []byte(`{"k":"v"}`))
}

func TestAssertJSONGolden_InvalidJSON_Fails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "invalid.json")
	p := probe()
	testutil.AssertJSONGolden(p, path, []byte(`not-json`))
	if !p.Failed() {
		t.Errorf("AssertJSONGolden with invalid JSON did not fail the probe")
	}
}
