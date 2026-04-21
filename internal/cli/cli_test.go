package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/example/go-cli-template/internal/cli"
	"github.com/example/go-cli-template/internal/output"
)

// isolatedEnv sets the environment variables tests rely on to known
// values, preventing host leakage. It uses t.Setenv so all writes are
// restored at test end. Pass empty string to disable file-config loading.
func isolatedEnv(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	// Null out any env var that would override a test flag; t.Setenv
	// restores the original on test end.
	t.Setenv("GO_CLI_TEMPLATE_CONFIG", "")
	t.Setenv("GO_CLI_TEMPLATE_OUTPUT", "")
	t.Setenv("GO_CLI_TEMPLATE_LOG_LEVEL", "")
	t.Setenv("GO_CLI_TEMPLATE_TIMEOUT", "")
	t.Setenv("GO_CLI_TEMPLATE_YES", "")
	t.Setenv("GO_CLI_TEMPLATE_DRY_RUN", "")
}

// errorEnvelope is the parsed shape of stderr in JSON error cases.
type errorEnvelope struct {
	Error struct {
		Code    string         `json:"code"`
		Message string         `json:"message"`
		Details map[string]any `json:"details,omitempty"`
	} `json:"error"`
}

func parseErrorEnvelope(t *testing.T, b []byte) errorEnvelope {
	t.Helper()
	var env errorEnvelope
	if err := json.Unmarshal(b, &env); err != nil {
		t.Fatalf("unmarshal error envelope %q: %v", string(b), err)
	}
	return env
}

func TestRun_BareRoot_ErrorsSubcommandRequired(t *testing.T) {
	isolatedEnv(t)

	var stdout, stderr bytes.Buffer
	code := cli.Run(
		context.Background(),
		cli.BuildInfo{Version: "test-v0.0.0"},
		[]string{},
		&stdout,
		&stderr,
	)

	if code != output.ExitUserError {
		t.Errorf("exit code = %d, want %d (stderr=%q)", code, output.ExitUserError, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout non-empty: %q", stdout.String())
	}
	env := parseErrorEnvelope(t, stderr.Bytes())
	if env.Error.Code != output.ErrCodeSubcommandRequired {
		t.Errorf("error.code = %q, want %q", env.Error.Code, output.ErrCodeSubcommandRequired)
	}
}

func TestRun_BareConfigGroup_ErrorsSubcommandRequired(t *testing.T) {
	isolatedEnv(t)

	var stdout, stderr bytes.Buffer
	code := cli.Run(
		context.Background(),
		cli.BuildInfo{Version: "test-v0.0.0"},
		[]string{"config"},
		&stdout,
		&stderr,
	)

	if code != output.ExitUserError {
		t.Errorf("exit code = %d, want %d (stderr=%q)", code, output.ExitUserError, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout non-empty: %q", stdout.String())
	}
	env := parseErrorEnvelope(t, stderr.Bytes())
	if env.Error.Code != output.ErrCodeSubcommandRequired {
		t.Errorf("error.code = %q, want %q", env.Error.Code, output.ErrCodeSubcommandRequired)
	}
}

func TestRun_InvalidOutputMode_Errors(t *testing.T) {
	isolatedEnv(t)

	var stdout, stderr bytes.Buffer
	code := cli.Run(
		context.Background(),
		cli.BuildInfo{Version: "test-v0.0.0"},
		[]string{"--output", "xml", "version"},
		&stdout,
		&stderr,
	)

	if code != output.ExitUserError {
		t.Errorf("exit code = %d, want %d (stderr=%q)", code, output.ExitUserError, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout non-empty: %q", stdout.String())
	}
	env := parseErrorEnvelope(t, stderr.Bytes())
	if env.Error.Code != output.ErrCodeInvalidOutputMode {
		t.Errorf("error.code = %q, want %q", env.Error.Code, output.ErrCodeInvalidOutputMode)
	}
}

func TestRun_Version_JSONMode_Succeeds(t *testing.T) {
	isolatedEnv(t)

	var stdout, stderr bytes.Buffer
	code := cli.Run(
		context.Background(),
		cli.BuildInfo{Version: "test-v0.0.0"},
		[]string{"version"},
		&stdout,
		&stderr,
	)

	if code != output.ExitSuccess {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, output.ExitSuccess, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr non-empty: %q", stderr.String())
	}

	var got struct {
		CLI struct {
			Version string `json:"version"`
		} `json:"cli"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal stdout %q: %v", stdout.String(), err)
	}
	if got.CLI.Version != "test-v0.0.0" {
		t.Errorf("cli.version = %q, want %q", got.CLI.Version, "test-v0.0.0")
	}
}
