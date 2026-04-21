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
// PersistentPreRunE and stored in the command's context.
type Deps struct {
	Flags  *Flags
	Build  BuildInfo
	Logger *slog.Logger
	HTTP   *http.Client
	Resty  *resty.Client
	Config *ConfigLayers
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

// Run builds the root command and executes it against args. Intended
// to be invoked from main; returns the process exit code.
func Run(ctx context.Context, bi BuildInfo, args []string) output.ExitCode {
	cmd := NewRoot(bi)
	cmd.SetArgs(args)
	cmd.SetContext(ctx)
	if err := cmd.Execute(); err != nil {
		return writeErrorAndExit(cmd, err)
	}
	return output.ExitSuccess
}

// NewRoot builds the cobra command tree. Exported so tests can drive
// individual subcommands without invoking os.Exit.
func NewRoot(bi BuildInfo) *cobra.Command {
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
			c.SetContext(context.WithValue(c.Context(), depsKey{}, deps))
			return nil
		},
	}
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
	// Logger — slog text handler to stderr. All logs go to stderr
	// regardless of output mode; stdout is reserved for command output.
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

	// HTTP + resty. Explicit *http.Client, per CLAUDE.md "HTTP client":
	// never resty.New() with defaults.
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

	// Config layers (precedence: flag > env > file > default).
	layers, err := loadConfig(f, pfs)
	if err != nil {
		return nil, err
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

// envKeyTransform maps GO_CLI_TEMPLATE_FOO_BAR → foo.bar.
func envKeyTransform(k, v string) (string, any) {
	k = strings.TrimPrefix(k, envVarPrefix)
	return strings.ReplaceAll(strings.ToLower(k), "_", "."), v
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

// writeErrorAndExit renders err to stderr in the current output mode
// and returns the process exit code. Best-effort mode detection: if the
// --output flag has been parsed, use its value; otherwise fall back to
// JSON (skills never assume human).
func writeErrorAndExit(cmd *cobra.Command, err error) output.ExitCode {
	mode := "json"
	if f := cmd.Root().PersistentFlags().Lookup("output"); f != nil {
		mode = f.Value.String()
	}
	output.WriteError(os.Stderr, mode, err)

	var oe *output.Error
	if errors.As(err, &oe) {
		return oe.ExitCode
	}
	return output.ExitUserError
}
