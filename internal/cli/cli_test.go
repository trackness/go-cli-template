package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
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

func TestRun_UnknownFlag_ErrorsInvalidFlag(t *testing.T) {
	isolatedEnv(t)

	var stdout, stderr bytes.Buffer
	code := cli.Run(
		context.Background(),
		cli.BuildInfo{Version: "test-v0.0.0"},
		[]string{"--foo"},
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
	if env.Error.Code != output.ErrCodeInvalidFlag {
		t.Errorf("error.code = %q, want %q", env.Error.Code, output.ErrCodeInvalidFlag)
	}
}

func TestRun_UnknownCommand_ErrorsUnknownCommand(t *testing.T) {
	isolatedEnv(t)

	var stdout, stderr bytes.Buffer
	code := cli.Run(
		context.Background(),
		cli.BuildInfo{Version: "test-v0.0.0"},
		[]string{"nonexistent-subcommand"},
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
	if env.Error.Code != output.ErrCodeUnknownCommand {
		t.Errorf("error.code = %q, want %q", env.Error.Code, output.ErrCodeUnknownCommand)
	}
}

func TestRun_Env_OutputMode_Backfills(t *testing.T) {
	isolatedEnv(t)
	t.Setenv("GO_CLI_TEMPLATE_OUTPUT", "human")

	var stdout, stderr bytes.Buffer
	code := cli.Run(
		context.Background(),
		cli.BuildInfo{Version: "test-v0.0.0"},
		[]string{"version"},
		&stdout,
		&stderr,
	)

	if code != output.ExitSuccess {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, output.ExitSuccess, stderr.String())
	}
	// Human-mode version prints "<cliName> <version>\n".
	want := "go-cli-template test-v0.0.0\n"
	if got := stdout.String(); got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
}

func TestRun_Env_LogLevel_KeyUsesDashes(t *testing.T) {
	isolatedEnv(t)
	t.Setenv("GO_CLI_TEMPLATE_LOG_LEVEL", "debug")

	var stdout, stderr bytes.Buffer
	code := cli.Run(
		context.Background(),
		cli.BuildInfo{Version: "test-v0.0.0"},
		[]string{"config", "view"},
		&stdout,
		&stderr,
	)

	if code != output.ExitSuccess {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, output.ExitSuccess, stderr.String())
	}

	var got struct {
		Values map[string]struct {
			Value  any    `json:"value"`
			Source string `json:"source"`
		} `json:"values"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v; stdout=%q", err, stdout.String())
	}
	v, ok := got.Values["log-level"]
	if !ok {
		t.Errorf("values[%q] missing; keys present: %v", "log-level", got.Values)
	}
	if v.Value != "debug" {
		t.Errorf("values[log-level].value = %v, want %q", v.Value, "debug")
	}
	if v.Source != "env" {
		t.Errorf("values[log-level].source = %q, want %q", v.Source, "env")
	}
}

func TestRun_Commands_Groups_HumanOutputFalse(t *testing.T) {
	isolatedEnv(t)

	var stdout, stderr bytes.Buffer
	code := cli.Run(
		context.Background(),
		cli.BuildInfo{Version: "test-v0.0.0"},
		[]string{"commands"},
		&stdout,
		&stderr,
	)

	if code != output.ExitSuccess {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, output.ExitSuccess, stderr.String())
	}

	var got struct {
		Commands []struct {
			Path        []string `json:"path"`
			HumanOutput bool     `json:"human_output"`
		} `json:"commands"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v; stdout=%q", err, stdout.String())
	}

	// Non-leaf commands (root with children, config group) cannot render
	// a human form — they dispatch or error with SUBCOMMAND_REQUIRED.
	// Introspection must agree with runtime so skills can trust the
	// human_output field.
	want := map[string]bool{"": false, "config": false}
	found := map[string]bool{}
	for _, cmd := range got.Commands {
		key := strings.Join(cmd.Path, "/")
		if _, ok := want[key]; ok {
			found[key] = true
			if cmd.HumanOutput {
				t.Errorf("path %q (group): human_output = true, want false", key)
			}
		}
	}
	for key := range want {
		if !found[key] {
			t.Errorf("expected path %q missing from commands output", key)
		}
	}
}

func TestRun_Commands_ConfigView_HumanOutputFalse(t *testing.T) {
	isolatedEnv(t)

	var stdout, stderr bytes.Buffer
	code := cli.Run(
		context.Background(),
		cli.BuildInfo{Version: "test-v0.0.0"},
		[]string{"commands"},
		&stdout,
		&stderr,
	)

	if code != output.ExitSuccess {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, output.ExitSuccess, stderr.String())
	}

	var got struct {
		Commands []struct {
			Path        []string `json:"path"`
			HumanOutput bool     `json:"human_output"`
		} `json:"commands"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v; stdout=%q", err, stdout.String())
	}

	var found bool
	for _, cmd := range got.Commands {
		if len(cmd.Path) == 2 && cmd.Path[0] == "config" && cmd.Path[1] == "view" {
			found = true
			if cmd.HumanOutput {
				t.Errorf("config view: human_output = true, want false (introspection must agree with runtime rejection)")
			}
			break
		}
	}
	if !found {
		t.Errorf("config view entry missing from commands output; got: %v", got.Commands)
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
