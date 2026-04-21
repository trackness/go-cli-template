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
	"github.com/example/go-cli-template/internal/testutil"
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
	// Echo the bad value back so skills can diagnose without re-parsing
	// the invocation — locks the message contract, not just the code.
	if !strings.Contains(env.Error.Message, `"xml"`) {
		t.Errorf("error.message = %q, want to contain %q", env.Error.Message, `"xml"`)
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

func TestRun_ConfigView_FiltersCLIOnlyKeys(t *testing.T) {
	// Two claims at once:
	//   1. CLI-layer flags (--output, --log-level, --timeout,
	//      --config, --yes, --dry-run, --no-retry) are filtered out
	//      of config view's Values map.
	//   2. Non-CLI env keys pass through with the flat-dash
	//      transform — GO_CLI_TEMPLATE_CUSTOM_THING lands at
	//      "custom-thing" (not "custom.thing"), exercising B2's
	//      transform. Name deliberately avoids sensitive suffixes
	//      (-key, -token, -secret, -password) so the redaction layer
	//      doesn't confound the assertion. Without this second claim
	//      a regression to dots would silently escape the filter
	//      test.
	isolatedEnv(t)
	t.Setenv("GO_CLI_TEMPLATE_LOG_LEVEL", "debug")
	t.Setenv("GO_CLI_TEMPLATE_OUTPUT", "json")
	t.Setenv("GO_CLI_TEMPLATE_YES", "true")
	t.Setenv("GO_CLI_TEMPLATE_CUSTOM_THING", "42")

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
	for _, cliKey := range []string{"output", "log-level", "timeout", "config", "yes", "dry-run", "no-retry"} {
		if _, present := got.Values[cliKey]; present {
			t.Errorf("values[%q] must not appear in config view (CLI-layer key); got keys %v", cliKey, got.Values)
		}
	}
	custom, ok := got.Values["custom-thing"]
	if !ok {
		t.Errorf("values[%q] missing; env key transform regressed? got keys %v", "custom-thing", got.Values)
	} else if custom.Value != "42" || custom.Source != "env" {
		t.Errorf("values[custom-thing] = %+v, want {Value: \"42\", Source: \"env\"}", custom)
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
	// JSON mode produces a parseable envelope with cli.version populated;
	// human mode emits plain text that won't unmarshal. The dual check
	// (parses AND field populated) beats a HasPrefix("{") coincidence.
	var parsed struct {
		CLI struct {
			Version string `json:"version"`
		} `json:"cli"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &parsed); err != nil {
		t.Fatalf("flag --output json did not win over env OUTPUT=human: stdout=%q not JSON: %v", stdout.String(), err)
	}
	if parsed.CLI.Version != "test-v0.0.0" {
		t.Errorf("cli.version = %q, want %q", parsed.CLI.Version, "test-v0.0.0")
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
	code := cli.Run(
		context.Background(),
		cli.BuildInfo{Version: "test-v0.0.0"},
		[]string{},
		&stdout,
		&stderr,
	)

	if code != output.ExitUserError {
		t.Errorf("exit code = %d, want %d", code, output.ExitUserError)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout non-empty: %q", stdout.String())
	}

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

func TestRun_ConfigView_RedactsSensitiveValues(t *testing.T) {
	isolatedEnv(t)
	t.Setenv("GO_CLI_TEMPLATE_TOKEN", "super-secret-value-xxx")

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

	tokenEntry, ok := got.Values["token"]
	if !ok {
		t.Fatalf("values[%q] missing; got keys %v", "token", got.Values)
	}
	if tokenEntry.Value != output.Redacted {
		t.Errorf("values[token].value = %v, want %q", tokenEntry.Value, output.Redacted)
	}
	// Source attribution must be preserved — skills still need to know
	// where the secret came from, just not what it was.
	if tokenEntry.Source != "env" {
		t.Errorf("values[token].source = %q, want %q", tokenEntry.Source, "env")
	}
	// Guard: the real value must not appear anywhere in the output.
	if strings.Contains(stdout.String(), "super-secret-value-xxx") {
		t.Errorf("secret value leaked into stdout: %q", stdout.String())
	}
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

// Golden tests pin the full JSON wire shape of contract-bearing
// commands. Regenerate after an intentional shape change:
//
//	go test ./internal/cli/... -update
//
// The field-level tests elsewhere in this file cover behavioural
// invariants; these tests fence byte-for-byte regressions skills
// would otherwise discover in production.

func TestRun_Version_Golden(t *testing.T) {
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
		t.Fatalf("exit code = %d (stderr=%q)", code, stderr.String())
	}
	testutil.AssertGolden(t, "testdata/version.json", stdout.Bytes())
}

func TestRun_Commands_Golden(t *testing.T) {
	// The CommandsOutput JSON shape is a versioned contract surface
	// (CLAUDE.md C6). Golden comparison catches any field add/remove
	// or reorder that field-level tests would miss.
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
		t.Fatalf("exit code = %d (stderr=%q)", code, stderr.String())
	}
	testutil.AssertGolden(t, "testdata/commands.json", stdout.Bytes())
}

func TestRun_ConfigView_Golden(t *testing.T) {
	// --config="" disables file loading and yields a deterministic
	// config_path_source of "flag-disabled" so the golden doesn't
	// embed a platform-varying XDG path.
	isolatedEnv(t)
	var stdout, stderr bytes.Buffer
	code := cli.Run(
		context.Background(),
		cli.BuildInfo{Version: "test-v0.0.0"},
		[]string{"--config=", "config", "view"},
		&stdout,
		&stderr,
	)
	if code != output.ExitSuccess {
		t.Fatalf("exit code = %d (stderr=%q)", code, stderr.String())
	}
	testutil.AssertGolden(t, "testdata/config_view_no_config.json", stdout.Bytes())
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
