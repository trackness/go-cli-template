package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/example/go-cli-template/internal/output"
)

// sterilizeEnv is the internal-package equivalent of isolatedEnv used
// by cli_test.go. Duplicated because internal-package tests need it
// for scenarios external-package tests cannot cover (constructing
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
// mutating command in the template; descendants declare their own.
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

// runWithMutating builds a root command with the mutating subcommand
// attached, then runs the full pipeline (runCmdTree → writeErrorAndExit
// on failure). Returns the exit code plus captured stdout/stderr so
// tests can assert on the stderr envelope the skill consumer sees.
func runWithMutating(t *testing.T, args []string) (stdout, stderr bytes.Buffer, code output.ExitCode) {
	t.Helper()
	cmd := NewRoot(BuildInfo{Version: "test-v0.0.0"}, &stdout, &stderr)
	cmd.AddCommand(newMutatingCmd())
	code = runCmdTree(context.Background(), cmd, args)
	return stdout, stderr, code
}

func TestMutationGuard_WithoutYesOrDryRun_EnvelopeAndExitCode(t *testing.T) {
	sterilizeEnv(t)

	stdout, stderr, code := runWithMutating(t, []string{"test-mutate"})

	if code != output.ExitUserError {
		t.Errorf("exit code = %d, want %d (stderr=%q)", code, output.ExitUserError, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout non-empty: %q", stdout.String())
	}

	var env struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stderr.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal stderr %q: %v", stderr.String(), err)
	}
	if env.Error.Code != output.ErrCodeConfirmationRequired {
		t.Errorf("error.code = %q, want %q", env.Error.Code, output.ErrCodeConfirmationRequired)
	}
	// Message must name the command path so skills can diagnose which
	// mutating subcommand rejected the invocation without re-parsing.
	if !strings.Contains(env.Error.Message, "test-mutate") {
		t.Errorf("error.message = %q, want to contain command path %q", env.Error.Message, "test-mutate")
	}
}

func TestMutationGuard_WithYes_Proceeds(t *testing.T) {
	sterilizeEnv(t)

	_, stderr, code := runWithMutating(t, []string{"test-mutate", "--yes"})
	if code != output.ExitSuccess {
		t.Errorf("exit code = %d, want %d (stderr=%q)", code, output.ExitSuccess, stderr.String())
	}
}

func TestMutationGuard_WithDryRun_Proceeds(t *testing.T) {
	sterilizeEnv(t)

	_, stderr, code := runWithMutating(t, []string{"test-mutate", "--dry-run"})
	if code != output.ExitSuccess {
		t.Errorf("exit code = %d, want %d (stderr=%q)", code, output.ExitSuccess, stderr.String())
	}
}

func TestRunCmdTree_PanicRecovered_StructuredEnvelope(t *testing.T) {
	// depsFromContext panics when called without PersistentPreRunE
	// having populated Deps — an invariant violation. A panic should
	// surface as the structured envelope skills branch on, not as a
	// Go stack trace on stderr.
	sterilizeEnv(t)

	var stdout, stderr bytes.Buffer
	cmd := NewRoot(BuildInfo{Version: "test-v0.0.0"}, &stdout, &stderr)
	cmd.AddCommand(&cobra.Command{
		Use: "test-panic",
		RunE: func(_ *cobra.Command, _ []string) error {
			panic("synthetic invariant violation")
		},
	})

	code := runCmdTree(context.Background(), cmd, []string{"test-panic"})

	if code != output.ExitUserError {
		t.Errorf("exit code = %d, want %d (stderr=%q)", code, output.ExitUserError, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout non-empty: %q", stdout.String())
	}
	var env struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stderr.Bytes(), &env); err != nil {
		t.Fatalf("stderr is not the structured envelope (likely a raw stack trace): %v; stderr=%q", err, stderr.String())
	}
	if env.Error.Code != output.ErrCodeUnknown {
		t.Errorf("error.code = %q, want %q", env.Error.Code, output.ErrCodeUnknown)
	}
	if !strings.Contains(env.Error.Message, "synthetic invariant violation") {
		t.Errorf("error.message = %q, want to contain panic value", env.Error.Message)
	}
}

func TestMutationGuard_UnannotatedCommand_Proceeds(t *testing.T) {
	// Without the annotation, version must run regardless of
	// --yes/--dry-run. The guard must only fire for opted-in commands.
	sterilizeEnv(t)

	_, stderr, code := runWithMutating(t, []string{"version"})
	if code != output.ExitSuccess {
		t.Errorf("exit code = %d, want %d (stderr=%q)", code, output.ExitSuccess, stderr.String())
	}
}
