package testutil

import (
	"bytes"
	"encoding/json"
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
//	go test ./... -update
//
// CLAUDE.md audience contract: any regeneration bumps the major
// version of the CLI and lands alongside matching prose edits.
//
// Package-level declaration: if a sibling test package declares a
// "update" flag of its own, flag.CommandLine will panic at init with
// "flag redefined". Reuse this helper rather than redeclaring.
var update = flag.Bool("update", false, "regenerate golden fixture files under testdata/")

// AssertGolden compares got against the bytes at path. On mismatch it
// reports a go-cmp diff via t.Errorf. When -update is set, got is
// written to path instead (creating parent directories as needed).
//
// Errorf (not Fatalf) is deliberate: testing.TB has a private method
// that prevents outside implementations, so probe tests of this
// helper rely on &testing.T{} whose state survives Errorf but not
// Fatalf's runtime.Goexit. The helper pairs every Errorf with a
// return to preserve the "on error, do nothing else" invariant.
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
			return
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

// AssertJSONGolden is the JSON-aware sibling of AssertGolden. It
// re-indents got through json.Indent before comparing/writing so
// failure diffs are line-oriented rather than character-offset. The
// template's production stdout stays compact — the indentation lives
// only in the fixture files where humans read diffs.
func AssertJSONGolden(t testing.TB, path string, got []byte) {
	t.Helper()
	var indented bytes.Buffer
	if err := json.Indent(&indented, got, "", "  "); err != nil {
		t.Errorf("indent got as JSON: %v (raw=%q)", err, string(got))
		return
	}
	indented.WriteByte('\n')
	AssertGolden(t, path, indented.Bytes())
}
