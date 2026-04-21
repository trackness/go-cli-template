package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/example/go-cli-template/internal/cli"
	"github.com/example/go-cli-template/internal/output"
)

// isolatedEnv sterilises the environment the CLI resolves config from
// so tests never read host state.
//
//   - XDG_CONFIG_HOME (anchor for config-file discovery on POSIX) and
//     HOME (POSIX fallback and Windows-adjacent) both go to temp dirs
//     so a regression in one still leaves no host-path to fall back to.
//   - Every GO_CLI_TEMPLATE_* env var currently set is blanked. Using
//     a prefix scan rather than a hand-maintained list keeps the helper
//     durable as descendants add new flags with env mappings.
//
// t.Setenv restores all writes at test end.
func isolatedEnv(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	for _, entry := range os.Environ() {
		if k, _, ok := strings.Cut(entry, "="); ok && strings.HasPrefix(k, "GO_CLI_TEMPLATE_") {
			t.Setenv(k, "")
		}
	}
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

func TestRun_Env_InvalidTimeout_Errors(t *testing.T) {
	isolatedEnv(t)
	t.Setenv("GO_CLI_TEMPLATE_TIMEOUT", "not-a-duration")

	var stdout, stderr bytes.Buffer
	code := cli.Run(
		context.Background(),
		cli.BuildInfo{Version: "test-v0.0.0"},
		[]string{"version"},
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

func TestRun_FlagBeatsEnv_OutputMode(t *testing.T) {
	// Flag > env per CLAUDE.md "Configuration". backfillFlags must only
	// overwrite the pflag value when pfs.Changed(name) is false.
	isolatedEnv(t)
	t.Setenv("GO_CLI_TEMPLATE_OUTPUT", "human")

	var stdout, stderr bytes.Buffer
	code := cli.Run(
		context.Background(),
		cli.BuildInfo{Version: "test-v0.0.0"},
		[]string{"--output", "json", "version"},
		&stdout,
		&stderr,
	)

	if code != output.ExitSuccess {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, output.ExitSuccess, stderr.String())
	}
	got := stdout.String()
	// JSON output starts with `{`; human output starts with CLI name.
	if !strings.HasPrefix(got, "{") {
		t.Errorf("flag --output json did not win over env OUTPUT=human: stdout=%q", got)
	}
}

func TestRun_HumanMode_ErrorIsPlainText(t *testing.T) {
	// Human-mode errors must render as plain "Error: <message>" on
	// stderr, never JSON. This path is the contract's human-mode
	// counterpart to the JSON envelope.
	isolatedEnv(t)

	var stdout, stderr bytes.Buffer
	code := cli.Run(
		context.Background(),
		cli.BuildInfo{Version: "test-v0.0.0"},
		[]string{"--output", "human"},
		&stdout,
		&stderr,
	)

	if code != output.ExitUserError {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, output.ExitUserError, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout non-empty: %q", stdout.String())
	}
	got := stderr.String()
	if !strings.HasPrefix(got, "Error: ") {
		t.Errorf("human-mode stderr %q does not start with %q", got, "Error: ")
	}
	if strings.Contains(got, `"code"`) {
		t.Errorf("human mode leaked JSON: %q", got)
	}
}

func TestRun_ErrorEnvelope_StrictStructure(t *testing.T) {
	// Envelope shape is a versioned contract surface. Top level has
	// exactly one key ("error"); body has code, message, and optional
	// details (omitted when empty). A regression adding a timestamp or
	// request-id sibling at top level would slip past tests that parse
	// into a typed struct — this test uses RawMessage to guard it.
	isolatedEnv(t)

	var stdout, stderr bytes.Buffer
	_ = cli.Run(
		context.Background(),
		cli.BuildInfo{Version: "test-v0.0.0"},
		[]string{},
		&stdout,
		&stderr,
	)

	var top map[string]json.RawMessage
	if err := json.Unmarshal(stderr.Bytes(), &top); err != nil {
		t.Fatalf("unmarshal top: %v; stderr=%q", err, stderr.String())
	}
	if len(top) != 1 {
		t.Errorf("envelope has %d top-level keys, want 1; keys=%v", len(top), keysOf(top))
	}
	if _, ok := top["error"]; !ok {
		t.Errorf("envelope missing %q key; keys=%v", "error", keysOf(top))
	}

	var body map[string]json.RawMessage
	if err := json.Unmarshal(top["error"], &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	for _, required := range []string{"code", "message"} {
		if _, ok := body[required]; !ok {
			t.Errorf("body missing required key %q; keys=%v", required, keysOf(body))
		}
	}
	// details must be omitted when empty (SUBCOMMAND_REQUIRED has no details)
	if _, ok := body["details"]; ok {
		t.Errorf("body contains %q but SUBCOMMAND_REQUIRED sets no details", "details")
	}
	for k := range body {
		if k != "code" && k != "message" && k != "details" {
			t.Errorf("body contains unexpected key %q", k)
		}
	}
}

func keysOf(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestRun_UnknownFlag_EnvOutputMode_RendersHuman(t *testing.T) {
	// Pre-parse errors (unknown flag) never reach PersistentPreRunE, so
	// backfillFlags doesn't run. writeErrorAndExit must still respect
	// the env-only human-mode request — otherwise env-configured skills
	// see JSON envelopes for exactly the errors they most need to read.
	isolatedEnv(t)
	t.Setenv("GO_CLI_TEMPLATE_OUTPUT", "human")

	var stdout, stderr bytes.Buffer
	code := cli.Run(
		context.Background(),
		cli.BuildInfo{Version: "test-v0.0.0"},
		[]string{"--foo"},
		&stdout,
		&stderr,
	)

	if code != output.ExitUserError {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", code, output.ExitUserError, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout non-empty: %q", stdout.String())
	}
	got := stderr.String()
	if !strings.HasPrefix(got, "Error: ") {
		t.Errorf("expected human-mode prefix %q, got %q", "Error: ", got)
	}
	if strings.Contains(got, `"code"`) {
		t.Errorf("human mode leaked JSON: %q", got)
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
