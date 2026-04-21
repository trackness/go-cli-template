# CLAUDE.md

Operating manual for this Go CLI. Per-repo context (what the CLI wraps,
endpoints, auth, domain vocabulary) lives in `PROJECT.md`. Repo-specific
content belongs there, not here.

Every domain-tool section follows the same template:

- **Default:** the chosen library (or stdlib) plus a one-line rationale
  when non-obvious.
- Bulleted rules governing use.
- **Opt-in** / **When opted in** / **Hard constraints** where a
  per-repo alternate is permitted (API client generation, mocking).
- **Excluded:** packages ruled out — with a reason when the reason is
  technical, bare names when the reason is just "picked one".

## Per-repo context

@./PROJECT.md

## Audience and output contract

The primary consumer of this CLI is a Claude skill, not a human at a terminal.
Output is an API. Treat every change to output shape as a breaking change.

- **Dual output modes.** JSON by default; human-readable with `--output
  human` (short `-o human`). No third mode unless `PROJECT.md` adds one.
  Unrecognised `--output` values exit `1` with `INVALID_OUTPUT_MODE`.
- **Default (JSON) mode is strict.** stdout contains a single JSON document
  (or NDJSON for streams) — nothing else. No ANSI, no progress bars, no
  banners. Logs go to stderr regardless of mode.
- **Help and usage routing.** Cobra's help and usage output is routed to
  stderr via `cmd.SetOut(os.Stderr)` on the root command. `--help` and
  `help` still dispatch through cobra's rendering, but land on stderr;
  stdout stays reserved for command output. A bare command group
  (root or any non-leaf) exits `1` with `SUBCOMMAND_REQUIRED` rather
  than dumping help to stdout.
- **Human mode is the exception.** `-o human` is the only mode where
  ANSI, colour, and Unicode box-drawing are permitted. Logs still go to
  stderr.
- **Human mode is optional per command.** A command may be declared
  machine-only when a rendered human form serves no realistic use
  (examples: `commands` introspection, raw diagnostic dumps). Invoking
  `-o human` on a machine-only command exits `1` with structured error
  code `HUMAN_OUTPUT_NOT_SUPPORTED`.
- **Deterministic output.** Sort collections by a stable key documented in
  the command's help. Omit timestamps from default output unless
  semantically required. Preserve struct field declaration order in JSON.
  Human-mode output is also deterministic — it is tested against golden
  text fixtures.
- **Stable exit-code taxonomy.** Skills branch on these; do not repurpose.
    0   success
    1   user / input error (bad flag, missing arg, missing required input,
        unsupported mode for this command)
    2   target system returned an error
    3   transport error (network, DNS, TLS, auth)
    130 SIGINT
- **Structured errors in default mode.** A single JSON object on stderr:
  `{"error": {"code": "<UPPER_SNAKE_CASE>", "message": "...", "details": {}}}`.
  Codes are stable across minor versions; adding a new code is additive.
- **Non-interactive, always.** No prompts. Mutating commands require `--yes`
  or `--dry-run`. If a TTY is not attached and input is missing, exit `1` with
  a structured error — never block waiting on stdin.
- **Paging discipline.** List commands default to a stated limit in `--help`.
  `--limit N` adjusts; `--all` is allowed with a stderr warning about output
  size. Never dump unbounded output to stdout by default.
- **Introspection.** A `commands` subcommand emits the full command tree
  with flags as JSON, so skills can discover the surface without parsing
  help text. The emitted `CommandsOutput` shape is a versioned contract
  surface; skills hard-code branches on its fields:
  - top level: `name`, `commands` (array), `exit_codes` (map), `error_codes`
    (array of stable upper-snake codes)
  - per command entry: `path` (array from root), `short`, `flags`,
    `human_output` (bool), `idempotent` (bool)
  Adding a field is additive; renaming or removing one is a major-version
  bump.
- **Versioning.** `version` emits the tool version. When the tool has reached
  a target system in the invocation, it also emits the target version seen.
- **Output changes are versioned.** Any change to JSON shape, error codes,
  exit-code meaning, or human-mode support bumps the major version and
  lands in CHANGELOG.

## Go version

Pinned in `go.mod`. This file assumes Go 1.26+ features are available.

## Layout

```
cmd/<cli-name>/       thin main — parse, construct, delegate
internal/cli/         command definitions, flag wiring, output formatting
internal/<domain>/    implementation packages, one concern each
internal/output/      json/human renderers, shared envelope types, exit codes
testdata/             golden files per package (human + json variants)
```

No `pkg/`. `internal/output/` owns the contract above; commands call into it
rather than rendering ad hoc.

## CLI framework

**Default:** `github.com/spf13/cobra` with `github.com/spf13/pflag`.

- One file per command under `internal/cli/`, each exporting a
  constructor returning `*cobra.Command`. Command *groups* and their
  *subcommands* are separate files: e.g. `config.go` holds the
  `config` group constructor, `config_view.go` holds `config view`.
  Any future group follows the same split; no exceptions.
- Composition is explicit in `internal/cli/root.go` — no `init()` for
  command registration.
- Persistent flags on the root: `--output/-o`, `--log-level`,
  `--timeout`, `--config`, `--yes`, `--dry-run`.
- Per-repo auth and endpoint flags live in `PROJECT.md` and are wired
  in `root.go`.

**Excluded:** `urfave/cli` (any version), `alecthomas/kong`.

## Configuration

Four sources, strict precedence: flag > env > file > default.

- Missing config file is **not** an error. Malformed config file **is** —
  exit `1` with a structured error naming the file path and parser message.
- Missing required values exit `1` with a structured error naming each
  missing key and the sources where it could be set.
- Auto-discovery, first match wins:
    1. `--config <path>` (explicit)
    2. `$<CLI>_CONFIG` env var
    3. `$XDG_CONFIG_HOME/<cli-name>/config.yaml`
    4. `$HOME/.config/<cli-name>/config.yaml` (fallback when XDG unset)
  On Windows, replace 3–4 with `%APPDATA%\<cli-name>\config.yaml`.
- `--config=""` (or env `$<CLI>_CONFIG=""`) disables file loading entirely.
  Skills that want hermetic invocation use this.
- A `config view` subcommand emits the resolved configuration with per-value
  source attribution (`flag`, `env`, `file:<path>`, `default`) as JSON.
- **Test isolation:** tests never read host config. Use `t.Setenv` to point
  `$XDG_CONFIG_HOME` at `t.TempDir()`, or pass `--config=""` explicitly.

**Default:** `github.com/knadh/koanf/v2` with providers `file`,
`env/v2`, `posflag`, and parser `yaml`.

**Excluded:** `spf13/viper` — divergent precedence semantics and
surprising behaviour around environment variables.

## Language & stdlib (Go 1.26)

Style rules across the codebase:

- `new(expr)` over `x := v; &x` for optional pointer fields.
- Stdlib builtins `min`, `max`, `clear` — do not import helpers.
- `context.Context` is the first parameter of every function that does
  I/O, is cancellable, or crosses a goroutine boundary. Name it `ctx`.
  Never store a context in a struct.
- Zero values are useful. Declare empty slices as `var s []T`.
  Exception: when a JSON response field must be `[]` not `null`,
  return `[]T{}` and comment.

### Logging

**Default:** `log/slog`; `slog.NewMultiHandler` when more than one
sink is required.

**Excluded:** `go.uber.org/zap`, `github.com/rs/zerolog`,
`github.com/sirupsen/logrus`.

### Atomics

**Default:** stdlib `sync/atomic` typed values (Go 1.19+ API).

**Excluded:** `go.uber.org/atomic` — stdlib typed atomics are a strict
superset.

### JSON

**Default:** stdlib `encoding/json`. `json/v2` is experimental in
1.26; do not enable.

**Excluded:** `json-iterator/go`.

### YAML

**Default:** `github.com/goccy/go-yaml` for any direct YAML use. goccy
honours `json` struct tags when `yaml` tags are absent, so the same
type serves both encoders without duplicate tagging.

**Excluded:** direct import of `gopkg.in/yaml.v3` (archived April
2025). koanf's default YAML parser still uses yaml.v3 transitively —
accepted compromise; revisit if koanf ships a goccy-backed parser or
yaml.v3 develops issues.

## HTTP client

**Default:** `github.com/go-resty/resty/v2` wrapping a single
`*http.Client` per CLI invocation.

- Configure in `root.go`; never use `resty.New()` with defaults — pass
  an explicit `*http.Client` so Transport and Timeout are under our
  control.
- Underlying `*http.Client.Timeout` is explicit (configurable via
  `--timeout`).
- `User-Agent` set at the resty level: `<cli-name>/<version> (+url)`.
- All requests use `R().SetContext(ctx)`; never background context.
- `EnableGenerateCurlOnDebug` is on at `--log-level=debug`, so any
  failed request can be reproduced from logs.
- Target-package code depends on narrow interfaces, not `*resty.Client`
  directly. The concrete `*resty.Client` is constructed once in
  `root.go` and passed into target constructors; narrow interfaces
  appear as parameters for unit-testable functions.
- Retry policy: see HTTP retry below. Per-target tuning lives in
  `PROJECT.md`.

**Excluded:** `net/http` `DefaultClient` / `DefaultTransport` — global
state leaks across invocations.

## HTTP retry

**Default:** resty's built-in retry, configured on the resty client in
`root.go`; no additional retry library.

- **Automatic retry applies to `GET` and `HEAD` only.** PUT, DELETE,
  POST, and PATCH are never auto-retried — the skill decides.
- **Retryable conditions:** HTTP `429`, `502`, `503`, `504`; transport
  errors (network, DNS, TLS handshake).
- **Defaults:** 3 attempts total (initial + 2 retries), exponential
  backoff with ±20% jitter, initial wait 100ms, per-attempt cap 5s.
  Total retry window is bounded by `--timeout` via
  `context.WithTimeout` on the root context — when the context fires,
  retries stop.
- **`Retry-After`** is honoured up to a 10s cap. If the server requests
  longer, fail immediately with a structured error rather than block.
- **`--no-retry`** disables all retries. Skills with their own retry
  layer set this to avoid double-retry.
- **Exit code mapping:** retries exhausted on transport error → exit
  `3`; retries exhausted on target error (or non-retryable 4xx/5xx) →
  exit `2`.
- **Target contract may extend the default.** If the target system
  documents stronger idempotency guarantees for specific methods or
  endpoints, `PROJECT.md` may broaden the retryable set with a cited
  justification. The default is deliberately conservative; extend it
  by design, not by drift.

**Excluded:** `hashicorp/go-retryablehttp`, `cenkalti/backoff`,
`avast/retry-go` — layering any over resty's own retry causes
double-retry bugs.

## API client generation

**Default:** hand-written client functions in `internal/<target>/`,
hand-written skill-facing types in `internal/output/`, conversion at
the boundary.

**Opt-in** to type-only codegen via
`github.com/oapi-codegen/oapi-codegen/v2` per repo when `PROJECT.md`
documents all three:

1. **Availability.** Official OpenAPI 3.0+ spec at a stable URL under
   target control. Community specs qualify only when no official
   version exists and quality is demonstrably higher.
2. **Quality.** Validates without errors on in-scope endpoints.
   Spot-check three endpoints against live target responses before
   trusting the spec; any discrepancy disqualifies.
3. **Scope.** Hand-written type definitions would exceed ~500 lines.
   Below that threshold, hand-written is cheaper to write, read, and
   maintain.

**When opted in:**

- `-generate types` only; `oapi-codegen/v2` version pinned.
- Generated code lives in `internal/<target>/gen/`; spec vendored at
  `internal/<target>/gen/spec.yaml`, committed.
- `//go:generate` drives regeneration. No network at generate time.
- Generated types are imported only by `internal/<target>/client.go` —
  never by `internal/output/`, never by `internal/cli/`.
- Regeneration is a deliberate PR citing the spec-version delta.

**Hard constraints** (whether opted in or not):

- Full client generation is ruled out; client functions are always
  hand-written so HTTP shape, auth, retry integration, and error
  mapping stay under direct control.
- Generated types never appear in skill-facing output schemas. The
  `internal/output/` boundary is always hand-written.
- Generated code never drives command structure. Commands are curated.

## Errors

**Default:** stdlib `errors` + `fmt.Errorf("<operation>: %w", err)`.

- Wrap with `fmt.Errorf("<operation>: %w", err)`; the message names the
  operation.
- Sentinel errors at package scope:
  `var ErrFoo = errors.New("foo")`. Export only if callers (or the
  error-code mapping) need to detect them.
- Detect with `errors.Is` / `errors.As`. No type switches on errors.
- Every error that reaches the user maps to a stable error code in
  `internal/output/errors.go`. New domain errors require a new code.
- Do not `panic` except in `main` for unrecoverable startup failures.

**Excluded:** `pkg/errors` — its `Cause()` semantics are incompatible
with `errors.Is` / `errors.As`.

## Concurrency

**Default:** stdlib `context`, `sync`, `sync/atomic`, `testing/synctest`;
`go.uber.org/goleak` in tests for packages that spawn goroutines.

- Every goroutine has an explicit lifecycle: `context.Context`
  cancellation and a wait mechanism. No fire-and-forget.
- Use `testing/synctest` for time-dependent concurrent tests. Never
  `time.Sleep` in tests.

**Excluded:** `jonboulle/clockwork`, `benbjohnson/clock` — fake-clock
libraries require production-code coupling that `synctest` eliminates.

## Human output rendering

Applies only when `-o human` is passed. Default JSON mode never
produces rendered output.

**Default:** `github.com/jedib0t/go-pretty/v6/table` at style `Light`
unless a command justifies another. Plain aligned columns
(shell-completion, key=value listings) may use stdlib `text/tabwriter`
when a bordered table is over-engineering.

- Output is deterministic — tested against golden text fixtures in
  `testdata/`.
- ANSI, colour, and Unicode box-drawing are permitted only in this
  mode. Colour choice is pinned in the TTY / colour section below.

**Excluded:** `github.com/olekukonko/tablewriter` — v1 rollout
unstable, legacy v0 API divergent.

## TTY / colour detection

Applies only in `-o human` mode.

Stderr errors in human mode: plain text `Error: ...`.

### TTY detection

**Default:** `golang.org/x/term`. Colour is emitted when stdout is a
TTY; suppressed otherwise.

### Table cell colour

**Default:** go-pretty's `text` subpackage (in via `go-pretty`
already). No separate library needed.

### General colour

**Default:** a ~20-line `internal/output/colour.go` hand-rolled helper
over `golang.org/x/term`. Adopt `github.com/fatih/color` at
implementation time only when three or more distinct colour uses arise
outside go-pretty tables; when adopted it replaces the hand-rolled
helper. Its transitive deps (`github.com/mattn/go-isatty`,
`github.com/mattn/go-colorable`) are accepted in that case.

**Excluded:** `github.com/charmbracelet/lipgloss`,
`github.com/muesli/termenv`; direct import of `mattn/go-isatty` or
`mattn/go-colorable` (transitive via `fatih/color` is fine).

## Testing

**Default:** stdlib `testing` + `github.com/google/go-cmp/cmp`.

- Table-driven tests with subtests (`t.Run(tt.name, ...)`). Named
  fields in the table literal when 4+ fields.
- Every command has a golden JSON fixture in `testdata/` for its
  default-mode output. Commands that implement `-o human` also have a
  golden text fixture. Output changes require a golden update and a
  CHANGELOG entry.
- `t.TempDir()` for filesystem work; never `os.TempDir()` directly.
- Integration tests behind `//go:build integration` with a separate
  `task` target.
- `go test -race ./...` must pass before a change is complete.

**Excluded:** `stretchr/testify` (any subpackage), `onsi/ginkgo`,
`onsi/gomega`.

## Mocking / test doubles

**Default:** hand-rolled fakes in test files. Define small focused
interfaces at command and client boundaries; implement fakes in the
test files that use them.

**Opt-in** to `go.uber.org/mock` (Uber's fork; `golang/mock` is
archived) per-repo for specific interfaces meeting both criteria,
documented in `PROJECT.md`:

1. **Size.** The interface has 10 or more methods.
2. **Recurrence.** Hand-rolled fakes for it would appear in 3 or more
   test files.

Opt-in is per-interface, not blanket. `PROJECT.md` names each
interface covered; all other interfaces stay hand-rolled.

**When opted in:**

- Tool: `go.uber.org/mock` with `mockgen`, version pinned.
- Generated mocks live in `internal/<target>/mocks/`, isolated from
  production code.
- `//go:generate` drives regeneration. Regeneration is a deliberate PR.

**Excluded:** `github.com/golang/mock` (archived), `vektra/mockery`
(any version), `matryer/moq`.

## HTTP response mocking

**Default:** stdlib `net/http/httptest` with a real local server. The
resty client's `BaseURL` is pointed at `httptest.Server.URL`; no
Transport-level interception.

- Handler functions inspect the incoming `*http.Request` directly
  (method, path, headers, body) and fail with a specific diff when
  expectations don't match. Use `go-cmp` on captured request fields.
- Canned responses live in `testdata/` alongside golden output
  fixtures, so fixture management stays in one place.
- A small `internal/testutil/` helper dispatches by `method + path` to
  keep multi-endpoint handlers readable. Written once, reused.
- Retry tests use a counter in the handler closure to return specific
  status codes in sequence.
- For TLS or mTLS scenarios, use `httptest.NewTLSServer` and configure
  the resty client's `*http.Client.Transport` accordingly in the test.

**Excluded:** `github.com/jarcoal/httpmock`, `github.com/h2non/gock`,
`github.com/dankinder/httpmock`, `go.nhat.io/httpmock` — each either
intercepts at the Transport level (conflicting with our
explicit-Transport rule) or couples to a mocking library already
banned.

## Lint & format

**Default:** `gofmt -s` and `goimports` on save; `golangci-lint` with
config at `.golangci.yml`.

- Findings are errors in CI.
- Disable a linter only inline with a comment explaining why.

## Verification loop

After any code change, run in order. Fix failures before presenting.

```
go fix ./...
gofmt -s -w .
go vet ./...
golangci-lint run
go test -race ./...
```

## Release tooling

Releases are driven by `goreleaser` v2 (OSS) on tag push. Local task
automation is driven by `go-task/task` via `Taskfile.yml`. Makefiles and
hand-rolled bash release scripts are not used.

### Taskfile

`Taskfile.yml` at the repo root. The CLI name is declared once as the
top-level `CLI_NAME` variable and referenced by the build task via
`{{.CLI_NAME}}`. Core tasks are `verify` (the full fix/format/vet/lint/test
loop), `test`, `lint`, `build`, and `release-snapshot`. Per-repo additions
go below the `# --- per-repo tasks below ---` marker at the end of the
file; do not interleave them with core tasks.

The `generate` task is included only when `PROJECT.md` declares a codegen
opt-in per API client generation or a mock opt-in per Mocking / test
doubles; otherwise omit it.

### goreleaser

`.goreleaser.yaml` at the repo root, `version: 2` header, OSS edition.
Target matrix: `darwin/amd64`, `darwin/arm64`, `linux/amd64`,
`linux/arm64`; archives are `tar.gz`; checksums are SHA-256 in
`checksums.txt`. The CLI name appears once as `project_name`; everything
else references `{{ .ProjectName }}`. Binary ldflags strip symbols and
inject `main.version`, `main.commit`, `main.date` at build time. The
changelog filter excludes `docs:`, `test:`, `chore:`, and merge commits.

### GitHub Actions workflow

`.github/workflows/release.yaml` triggers on `v*.*.*` tag push. Uses
`actions/checkout@v4` with `fetch-depth: 0`, `actions/setup-go@v5`
keyed off `go.mod`, and `goreleaser/goreleaser-action@v6` with
`args: release --clean`. Requires `contents: write`; passes
`GITHUB_TOKEN` through to goreleaser.

### Release flow

1. `task verify` locally.
2. Optionally `task release-snapshot` to exercise the build without
   publishing.
3. `git tag vX.Y.Z && git push origin vX.Y.Z`.
4. Actions builds, cross-compiles, publishes to the tag's Release page.

Tag format is `vMAJOR.MINOR.PATCH`. Output-contract breaking changes bump
MAJOR, per the Audience and output contract section.

## Dependencies

- Prefer stdlib. A new third-party dependency requires a one-line
  justification in the PR description naming the stdlib alternative
  and why it was insufficient.
- Allowed by default: `github.com/spf13/cobra`,
  `github.com/spf13/pflag`, `github.com/knadh/koanf/v2` (with
  providers `file`, `env/v2`, `posflag` and parser `yaml`),
  `github.com/go-resty/resty/v2`, `github.com/goccy/go-yaml`,
  `github.com/jedib0t/go-pretty/v6`, `golang.org/x/term`,
  `github.com/google/go-cmp/cmp` (tests only),
  `go.uber.org/goleak` (tests only). Anything else needs
  justification.
- Exclusions are colocated with the positive rule in each relevant
  section. No separate banned list.

## Working style in this repo

- State assumptions inline before writing code, not after.
- Don't silently refactor working code for style. Name the motivation
  first and wait for a yes.
- Don't revert code based on a search result or style preference. If
  something looks wrong, ask — working code stays until there's a real
  reason.
- On architectural branchpoints (new package, new dependency, new
  exported API, new error code, new output field), ask one clarifying
  question rather than guessing.
- Do not add vague rules like "follow best practices" when editing
  this file.
