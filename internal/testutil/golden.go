package testutil

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// update is the -update flag. When set, AssertGolden writes `got` to
// disk instead of comparing; use it to regenerate fixtures after an
// intentional output-shape change:
//
//	go test ./internal/cli/... -update
//
// CLAUDE.md audience contract: any regeneration bumps the major
// version of the CLI and lands alongside matching prose edits.
var update = flag.Bool("update", false, "regenerate golden fixture files under testdata/")

// AssertGolden compares got against the bytes at path. On mismatch it
// reports a go-cmp diff via t.Errorf. When -update is set, got is
// written to path instead (creating parent directories as needed).
//
// Path is conventionally rooted at the caller's package testdata/
// directory, e.g. "testdata/version.json" when called from a test in
// internal/cli/.
func AssertGolden(t testing.TB, path string, got []byte) {
	t.Helper()
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Errorf("mkdir %s: %v", filepath.Dir(path), err)
			return
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Errorf("write golden %s: %v", path, err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Errorf("read golden %s: %v (run with -update to create)", path, err)
		return
	}
	if diff := cmp.Diff(string(want), string(got)); diff != "" {
		t.Errorf("golden %s mismatch (-want +got):\n%s", path, diff)
	}
}
