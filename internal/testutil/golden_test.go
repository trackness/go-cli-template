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

func TestAssertGolden_Mismatch_FailsTest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mismatch.txt")
	if err := os.WriteFile(path, []byte("expected\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	fake := &testing.T{}
	testutil.AssertGolden(fake, path, []byte("actual\n"))
	if !fake.Failed() {
		t.Errorf("AssertGolden with mismatched content did not fail the test")
	}
}

func TestAssertGolden_MissingFile_FailsTest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.txt")
	fake := &testing.T{}
	testutil.AssertGolden(fake, path, []byte("anything"))
	if !fake.Failed() {
		t.Errorf("AssertGolden with missing golden file did not fail the test")
	}
}
