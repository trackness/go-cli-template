// Package cli wires the cobra command tree, constructs the shared
// dependencies (slog logger, resty client, koanf config layers), and
// exposes the entry point Run.
//
// Rename the cliName and envVarPrefix constants below when forking.
// Every occurrence of "go-cli-template" (lowercase-kebab) and
// "GO_CLI_TEMPLATE" (upper-snake) in the tree derives from these two
// constants, either directly or via find/replace.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/knadh/koanf/parsers/yaml"
	env "github.com/knadh/koanf/providers/env/v2"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/posflag"
	"github.com/knadh/koanf/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/example/go-cli-template/internal/output"
)

// Template identity. Replace both when forking.
const (
	cliName      = "go-cli-template"
	envVarPrefix = "GO_CLI_TEMPLATE_"
)

// Cobra annotation keys. The prefix matches the CLI name so they don't
// collide with annotations from third-party cobra integrations.
const (
	annotationMachineOnly = cliName + "/machine-only"
	annotationIdempotent  = cliName + "/idempotent"
	annotationMutating    = cliName + "/mutating"
)

// BuildInfo carries compile-time version metadata injected via -ldflags
// in goreleaser.
type BuildInfo struct {
	Version string `json:"version"`
	Commit  string `json:"commit,omitempty"`
	Date    string `json:"date,omitempty"`
}

// Flags holds the persistent flag values parsed on the root command.
// The lowercase fields are populated during config resolution and read
// by the `config view` subcommand.
type Flags struct {
	Output     string
	LogLevel   string
	Timeout    time.Duration
	ConfigPath string
	Yes        bool
	DryRun     bool

	configPath   string
	configSource string
}

// Deps is the bag of shared dependencies constructed in
// PersistentPreRunE and stored in the command's context. Stdout and
// Stderr are the writers threaded through from Run so commands (and
// tests) never touch os.Stdout/os.Stderr directly.
type Deps struct {
	Flags  *Flags
	Build  BuildInfo
	Logger *slog.Logger
	HTTP   *http.Client
	Resty  *resty.Client
	Config *ConfigLayers
	Stdout io.Writer
	Stderr io.Writer
}

// ConfigLayers holds each source's koanf instance separately so that
// `config view` can attribute each resolved value to its origin.
type ConfigLayers struct {
	Merged   *koanf.Koanf
	Flags    *koanf.Koanf
	Env      *koanf.Koanf
	File     *koanf.Koanf
	Defaults *koanf.Koanf
	FilePath string
}

type depsKey struct{}

// Run builds the root command and executes it against args. stdout and
// stderr are threaded through to subcommands via Deps; pass os.Stdout
// and os.Stderr from main, bytes.Buffer from tests.
func Run(ctx context.Context, bi BuildInfo, args []string, stdout, stderr io.Writer) output.ExitCode {
	return runCmdTree(ctx, NewRoot(bi, stdout, stderr), args)
}

// runCmdTree executes cmd against args and routes any error through
// writeErrorAndExit — the same pipeline Run uses. Extracted so
// internal tests can inject extra subcommands into the tree before
// Execute and still exercise the full error-rendering path.
func runCmdTree(ctx context.Context, cmd *cobra.Command, args []string) output.ExitCode {
	cmd.SetArgs(args)
	cmd.SetContext(ctx)
	if err := cmd.Execute(); err != nil {
		return writeErrorAndExit(cmd, err)
	}
	return output.ExitSuccess
}

// NewRoot builds the cobra command tree. Exported so tests can drive
// individual subcommands without invoking os.Exit. stdout and stderr
// are routed to Deps.Stdout / Deps.Stderr and (for cobra's own help and
// error output) via cmd.SetOut / cmd.SetErr.
func NewRoot(bi BuildInfo, stdout, stderr io.Writer) *cobra.Command {
	flags := &Flags{}

	cmd := &cobra.Command{
		Use:           cliName,
		Short:         "Template for Go CLIs wrapping REST target systems",
		SilenceUsage:  true,
		SilenceErrors: true,
		// Cobra's auto-generated `completion` subcommand is not
		// relevant to skill consumers; hiding it keeps `commands`
		// introspection focused on the real surface.
		CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
		PersistentPreRunE: func(c *cobra.Command, _ []string) error {
			deps, err := buildDeps(bi, flags, c.Flags())
			if err != nil {
				return err
			}
			deps.Stdout = stdout
			deps.Stderr = stderr

			// Mutation guard: any command marked with
			// annotationMutating must receive --yes or --dry-run. The
			// check runs after buildDeps so deps.Flags reflects the
			// resolved values (flag > env > file > default) rather than
			// pflag's pre-backfill state.
			if c.Annotations[annotationMutating] == "true" && !deps.Flags.Yes && !deps.Flags.DryRun {
				return &output.Error{
					Code:     output.ErrCodeConfirmationRequired,
					Message:  fmt.Sprintf("%q mutates state; pass --yes to proceed or --dry-run to preview", c.CommandPath()),
					ExitCode: output.ExitUserError,
				}
			}

			c.SetContext(context.WithValue(c.Context(), depsKey{}, deps))
			return nil
		},
		RunE: subcommandRequiredRunE,
	}
	// Route cobra's help and usage to stderr; stdout is reserved for
	// command output. SetErr is explicit so writeErrorAndExit can read
	// cmd.ErrOrStderr() regardless of whether cobra is also writing.
	cmd.SetOut(stderr)
	cmd.SetErr(stderr)
	registerPersistentFlags(cmd, flags)

	cmd.AddCommand(newVersionCommand())
	cmd.AddCommand(newCommandsCommand(cmd))
	cmd.AddCommand(newConfigCommand())

	return cmd
}

func registerPersistentFlags(cmd *cobra.Command, f *Flags) {
	fs := cmd.PersistentFlags()
	fs.StringVarP(&f.Output, "output", "o", "json", "output mode: json or human")
	fs.StringVar(&f.LogLevel, "log-level", "info", "log level: debug, info, warn, error")
	fs.DurationVar(&f.Timeout, "timeout", 30*time.Second, "overall request timeout")
	fs.StringVar(&f.ConfigPath, "config", "", "config file path; unset auto-discovers, empty disables file loading")
	fs.BoolVar(&f.Yes, "yes", false, "assume yes for confirmations; required for mutating commands")
	fs.BoolVar(&f.DryRun, "dry-run", false, "dry-run: mutating commands do not mutate")
}

// depsFromContext retrieves Deps populated by PersistentPreRunE.
// Panics if called before PersistentPreRunE — subcommands must always
// be invoked via the root command.
func depsFromContext(ctx context.Context) *Deps {
	d, _ := ctx.Value(depsKey{}).(*Deps)
	if d == nil {
		panic("cli: Deps missing; PersistentPreRunE must run before subcommands")
	}
	return d
}

func buildDeps(bi BuildInfo, f *Flags, pfs *pflag.FlagSet) (*Deps, error) {
	// 1. Load config layers (precedence: flag > env > file > default).
	layers, err := loadConfig(f, pfs)
	if err != nil {
		return nil, err
	}

	// 2. Backfill *Flags fields from the merged layers when pflag did
	//    not explicitly set them. Realises the documented precedence on
	//    the struct commands read from. --config itself is not
	//    backfilled (resolveConfigPath already consumed it).
	if err := backfillFlags(f, pfs, layers.Merged); err != nil {
		return nil, err
	}

	// 3. Validate --output on the final resolved value. An unsupported
	//    mode fails fast with a stable code rather than silently
	//    falling through to default JSON.
	if f.Output != "json" && f.Output != "human" {
		return nil, &output.Error{
			Code:     output.ErrCodeInvalidOutputMode,
			Message:  fmt.Sprintf("invalid --output %q (want json or human)", f.Output),
			ExitCode: output.ExitUserError,
		}
	}

	// 4. Logger — slog text handler to stderr. All logs go to stderr
	//    regardless of output mode; stdout is reserved for command output.
	var level slog.Level
	if err := level.UnmarshalText([]byte(f.LogLevel)); err != nil {
		return nil, &output.Error{
			Code:     output.ErrCodeInvalidLogLevel,
			Message:  fmt.Sprintf("invalid --log-level %q (want debug, info, warn, or error)", f.LogLevel),
			ExitCode: output.ExitUserError,
			Cause:    err,
		}
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	// 5. HTTP + resty. Explicit *http.Client, per CLAUDE.md "HTTP client":
	//    never resty.New() with defaults.
	httpClient := &http.Client{Timeout: f.Timeout}
	r := resty.NewWithClient(httpClient).
		SetHeader("User-Agent", fmt.Sprintf("%s/%s", cliName, bi.Version)).
		SetRetryCount(2).
		SetRetryWaitTime(100 * time.Millisecond).
		SetRetryMaxWaitTime(5 * time.Second).
		AddRetryCondition(retryCondition)
	if level == slog.LevelDebug {
		r.EnableGenerateCurlOnDebug()
	}

	return &Deps{
		Flags:  f,
		Build:  bi,
		Logger: logger,
		HTTP:   httpClient,
		Resty:  r,
		Config: layers,
	}, nil
}

// backfillFlags copies resolved values from the merged config layers
// onto f for any flag the user did not explicitly set on the command
// line. --config is deliberately skipped because resolveConfigPath
// already consumed it.
//
// For strings, empty values in the merged layers are treated as unset
// (the env transform already filters exported-but-blank env vars; this
// handles the same case arriving from a config file). For bools and
// durations, presence is decided by koanf's Exists so a stringified
// "false" isn't mistaken for absence.
//
// An unparseable timeout returns a structured INVALID_FLAG error
// rather than silently falling back to the pflag default.
func backfillFlags(f *Flags, pfs *pflag.FlagSet, merged *koanf.Koanf) error {
	if !pfs.Changed("output") {
		if v := merged.String("output"); v != "" {
			f.Output = v
		}
	}
	if !pfs.Changed("log-level") {
		if v := merged.String("log-level"); v != "" {
			f.LogLevel = v
		}
	}
	if !pfs.Changed("timeout") && merged.Exists("timeout") {
		v := merged.String("timeout")
		d, err := time.ParseDuration(v)
		if err != nil {
			return &output.Error{
				Code:     output.ErrCodeInvalidFlag,
				Message:  fmt.Sprintf("invalid timeout %q: %v", v, err),
				ExitCode: output.ExitUserError,
				Cause:    err,
			}
		}
		f.Timeout = d
	}
	if !pfs.Changed("yes") && merged.Exists("yes") {
		f.Yes = merged.Bool("yes")
	}
	if !pfs.Changed("dry-run") && merged.Exists("dry-run") {
		f.DryRun = merged.Bool("dry-run")
	}
	return nil
}

// retryCondition enforces the contract-mandated retry policy: GET and
// HEAD only; status 429/502/503/504 or transport error. PUT, POST,
// DELETE, and PATCH are never auto-retried — the skill decides.
func retryCondition(resp *resty.Response, err error) bool {
	if resp == nil {
		return err != nil
	}
	req := resp.Request
	if req == nil {
		return false
	}
	if req.Method != http.MethodGet && req.Method != http.MethodHead {
		return false
	}
	switch resp.StatusCode() {
	case http.StatusTooManyRequests,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	}
	return false
}

// loadConfig assembles the per-source koanf instances plus their merged
// view. Per-source instances are retained so that `config view` can
// attribute each value to its origin.
func loadConfig(f *Flags, pfs *pflag.FlagSet) (*ConfigLayers, error) {
	defaults := koanf.New(".")
	// No template-level defaults. Per-repo code installs defaults here
	// (before the other layers are merged) as needed.

	path, source := resolveConfigPath(pfs, f.ConfigPath)
	f.configPath = path
	f.configSource = source

	fileLayer := koanf.New(".")
	if path != "" {
		if loadErr := fileLayer.Load(file.Provider(path), yaml.Parser()); loadErr != nil {
			if !errors.Is(loadErr, os.ErrNotExist) {
				return nil, &output.Error{
					Code:     output.ErrCodeMalformedConfigFile,
					Message:  fmt.Sprintf("config file %q failed to parse", path),
					Details:  map[string]any{"path": path},
					ExitCode: output.ExitUserError,
					Cause:    loadErr,
				}
			}
			// Missing file is not an error per CLAUDE.md; annotate the
			// source string so `config view` surfaces it.
			f.configSource = source + "-missing"
		}
	}

	envLayer := koanf.New(".")
	if err := envLayer.Load(env.Provider(".", env.Opt{
		Prefix:        envVarPrefix,
		TransformFunc: envKeyTransform,
	}), nil); err != nil {
		return nil, fmt.Errorf("load env config: %w", err)
	}

	// posflag with ko=nil: only Changed flags are included. Unchanged
	// flags (still at default) are left to earlier layers.
	flagsLayer := koanf.New(".")
	if err := flagsLayer.Load(posflag.Provider(pfs, ".", nil), nil); err != nil {
		return nil, fmt.Errorf("load flag config: %w", err)
	}

	merged := koanf.New(".")
	for _, layer := range []*koanf.Koanf{defaults, fileLayer, envLayer, flagsLayer} {
		if err := merged.Merge(layer); err != nil {
			return nil, fmt.Errorf("merge config layer: %w", err)
		}
	}

	return &ConfigLayers{
		Merged:   merged,
		Flags:    flagsLayer,
		Env:      envLayer,
		File:     fileLayer,
		Defaults: defaults,
		FilePath: path,
	}, nil
}

// envKeyTransform maps GO_CLI_TEMPLATE_FOO_BAR → foo-bar. Keys are
// flat with dashes to align with pflag's posflag provider: the env and
// flag layers then share a single namespace so merge precedence works.
// Nested-config env mapping is deliberately out of scope; descendant
// repos that need it add their own mapper.
//
// Empty values are skipped (returning an empty key tells the koanf env
// provider to drop the entry). This treats `VAR=""` as "unset" so an
// exported-but-blank env var does not clobber a pflag default.
func envKeyTransform(k, v string) (string, any) {
	if v == "" {
		return "", nil
	}
	k = strings.TrimPrefix(k, envVarPrefix)
	return strings.ReplaceAll(strings.ToLower(k), "_", "-"), v
}

// resolveConfigPath applies the discovery rule from CLAUDE.md. The
// returned source string is stable and consumed by `config view`:
//
//	flag           — explicit --config <path>
//	flag-disabled  — explicit --config="" (file loading off)
//	env            — value from $<PREFIX>CONFIG
//	env-disabled   — explicit $<PREFIX>CONFIG="" (file loading off)
//	default-xdg    — $XDG_CONFIG_HOME/<cli>/config.yaml
//	default-appdata — %APPDATA%\<cli>\config.yaml (Windows)
//	default-home   — $HOME/.config/<cli>/config.yaml (POSIX fallback)
//	none           — no discoverable location
func resolveConfigPath(pfs *pflag.FlagSet, explicit string) (path, source string) {
	if cfgFlag := pfs.Lookup("config"); cfgFlag != nil && cfgFlag.Changed {
		if explicit == "" {
			return "", "flag-disabled"
		}
		return explicit, "flag"
	}

	if v, ok := os.LookupEnv(envVarPrefix + "CONFIG"); ok {
		if v == "" {
			return "", "env-disabled"
		}
		return v, "env"
	}

	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, cliName, "config.yaml"), "default-xdg"
	}

	if runtime.GOOS == "windows" {
		if ap := os.Getenv("APPDATA"); ap != "" {
			return filepath.Join(ap, cliName, "config.yaml"), "default-appdata"
		}
	} else {
		if home, herr := os.UserHomeDir(); herr == nil {
			return filepath.Join(home, ".config", cliName, "config.yaml"), "default-home"
		}
	}

	return "", "none"
}

// subcommandRequiredError is the error returned when a command group
// (root, `config`, etc.) is invoked without a subcommand. Skills branch
// on ErrCodeSubcommandRequired to know to re-dispatch with one.
func subcommandRequiredError(path string) *output.Error {
	return &output.Error{
		Code:     output.ErrCodeSubcommandRequired,
		Message:  fmt.Sprintf("%q requires a subcommand; see %q --help", path, path),
		ExitCode: output.ExitUserError,
	}
}

// subcommandRequiredRunE is the shared RunE for any command group —
// root, `config`, and any future non-leaf command. Consolidates what
// would otherwise be two inline closures with the same body.
func subcommandRequiredRunE(c *cobra.Command, _ []string) error {
	return subcommandRequiredError(c.CommandPath())
}

// newGroupCommand constructs a cobra.Command for a command group — a
// non-leaf command that exists only to hold subcommands. Bare
// invocation of the group errors with SUBCOMMAND_REQUIRED rather than
// dumping help to stdout.
func newGroupCommand(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		RunE:  subcommandRequiredRunE,
	}
}

// writeErrorAndExit renders err to the command's configured stderr in
// the current output mode and returns the process exit code. Best-
// effort mode detection: if the --output flag has been parsed, use its
// value; otherwise fall back to JSON (skills never assume human).
func writeErrorAndExit(cmd *cobra.Command, err error) output.ExitCode {
	mode := resolveErrorMode(cmd)

	// Cobra surfaces "unknown flag" and "unknown command" as plain
	// errors without typed sentinels. Map them to stable codes by
	// message prefix before rendering so skills branch on a code rather
	// than reading prose. Skip the mapping when err is already
	// structured — the mapper assumes an unstructured input.
	var oe *output.Error
	if !errors.As(err, &oe) {
		if mapped := mapCobraNativeError(err); mapped != nil {
			err = mapped
		}
	}

	output.WriteError(cmd.ErrOrStderr(), mode, err)

	if errors.As(err, &oe) {
		return oe.ExitCode
	}
	return output.ExitUserError
}

// resolveErrorMode picks the output mode to render an error with when
// writeErrorAndExit is invoked. Precedence matches the resolved config
// precedence (flag > env > default) but avoids touching Deps because
// PersistentPreRunE may not have run (e.g. pflag parse failed before
// buildDeps could resolve anything). Values other than json or human
// from either source fall through to json — invalid user input never
// dictates error rendering.
func resolveErrorMode(cmd *cobra.Command) string {
	if f := cmd.Root().PersistentFlags().Lookup("output"); f != nil && f.Changed {
		if v := f.Value.String(); v == "json" || v == "human" {
			return v
		}
	}
	if v := os.Getenv(envVarPrefix + "OUTPUT"); v == "json" || v == "human" {
		return v
	}
	return "json"
}

// mapCobraNativeError matches cobra's unstructured flag/command errors
// by message prefix and wraps them in *output.Error with a stable
// code. Returns nil when no prefix matches. Caller (writeErrorAndExit)
// guarantees err is not already a *output.Error.
//
// Coverage is deliberately narrow — only the cobra/pflag errors skills
// commonly branch on are mapped. Other pflag typed errors
// (ValueRequiredError, InvalidValueError, InvalidSyntaxError) fall
// through to UNKNOWN; extend here if a skill needs to distinguish
// them. Locked against upstream message-wording drift by
// TestMapCobraNativeError_KnownPrefixes.
func mapCobraNativeError(err error) *output.Error {
	msg := err.Error()
	switch {
	case strings.HasPrefix(msg, "unknown flag:"),
		strings.HasPrefix(msg, "unknown shorthand flag:"),
		strings.HasPrefix(msg, "flag provided but not defined:"):
		return &output.Error{
			Code:     output.ErrCodeInvalidFlag,
			Message:  msg,
			ExitCode: output.ExitUserError,
		}
	case strings.HasPrefix(msg, "unknown command"):
		return &output.Error{
			Code:     output.ErrCodeUnknownCommand,
			Message:  msg,
			ExitCode: output.ExitUserError,
		}
	}
	return nil
}
