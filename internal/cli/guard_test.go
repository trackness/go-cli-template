package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/example/go-cli-template/internal/output"
)

// sterilizeEnv is the internal-package equivalent of isolatedEnv used
// by cli_test.go. Duplicated because internal-package tests need it
// for scenarios that external-package tests cannot cover (constructing
// commands with unexported annotation keys).
func sterilizeEnv(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	for _, entry := range os.Environ() {
		if k, _, ok := strings.Cut(entry, "="); ok && strings.HasPrefix(k, envVarPrefix) {
			t.Setenv(k, "")
		}
	}
}

// newMutatingCmd builds a throwaway subcommand carrying the mutating
// annotation. Used to exercise the mutation guard without shipping a
// mutating command in the template (descendants declare their own).
func newMutatingCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "test-mutate",
		Short: "Test-only mutating command",
		Annotations: map[string]string{
			annotationMutating: "true",
		},
		RunE: func(c *cobra.Command, _ []string) error { return nil },
	}
}

func runWithMutating(t *testing.T, args []string) error {
	t.Helper()
	var stdout, stderr bytes.Buffer
	cmd := NewRoot(BuildInfo{Version: "test-v0.0.0"}, &stdout, &stderr)
	cmd.AddCommand(newMutatingCmd())
	cmd.SetArgs(args)
	cmd.SetContext(context.Background())
	return cmd.Execute()
}

func TestMutationGuard_WithoutYesOrDryRun_Errors(t *testing.T) {
	sterilizeEnv(t)

	err := runWithMutating(t, []string{"test-mutate"})
	if err == nil {
		t.Fatal("Execute returned nil, want *output.Error with CONFIRMATION_REQUIRED")
	}
	var oe *output.Error
	if !errors.As(err, &oe) {
		t.Fatalf("err is not *output.Error: %v", err)
	}
	if oe.Code != output.ErrCodeConfirmationRequired {
		t.Errorf("code = %q, want %q", oe.Code, output.ErrCodeConfirmationRequired)
	}
	if oe.ExitCode != output.ExitUserError {
		t.Errorf("exit code = %d, want %d", oe.ExitCode, output.ExitUserError)
	}
}

func TestMutationGuard_WithYes_Proceeds(t *testing.T) {
	sterilizeEnv(t)

	if err := runWithMutating(t, []string{"test-mutate", "--yes"}); err != nil {
		t.Errorf("Execute with --yes returned error: %v", err)
	}
}

func TestMutationGuard_WithDryRun_Proceeds(t *testing.T) {
	sterilizeEnv(t)

	if err := runWithMutating(t, []string{"test-mutate", "--dry-run"}); err != nil {
		t.Errorf("Execute with --dry-run returned error: %v", err)
	}
}

func TestMutationGuard_NonMutatingCommand_NoGuard(t *testing.T) {
	// Without the annotation, version must run regardless of --yes/--dry-run.
	sterilizeEnv(t)

	if err := runWithMutating(t, []string{"version"}); err != nil {
		t.Errorf("non-mutating command blocked without --yes: %v", err)
	}
}
