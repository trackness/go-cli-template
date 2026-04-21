# Handoff — go-cli-template assessment

*Transient artifact. Do not commit to a descendant repo. Delete once acted on.*

A prior Claude Code session assessed this repo against its own `CLAUDE.md`
contract. This file is the stand-alone summary, written so a fresh session
can resume without re-deriving.

Read order: this file, then `CLAUDE.md`, then `PROJECT.md`. The user has
already read the assessment; they want work, not re-summary.

---

## Brief

**What the user asked:**

> "I want you to assess the whole existing repo, CLAUDE.md included. The
> library decisions in CLAUDE.md are not up for debate. Everything else
> is. Be thorough. I do NOT want to have to drag you through this."

**Why the list exists:**

This template is the source from which real Go CLIs are cloned. Every
gap between `CLAUDE.md`'s contract and the template's actual
implementation becomes a silent defect in every descendant CLI —
shipped to the skill consumers that are the primary audience. The
findings in §5 exist to close those gaps at the source so descendants
are born contract-compliant, instead of inheriting unfixed regressions
at fork time. "Be thorough, don't drag me through it" is the working
posture the user expects of the resuming session.

---

## 1. What this repo is

A template for Go CLIs whose **primary consumer is a Claude skill**, not a
human. Clone per target system; `CLAUDE.md` copies verbatim; `PROJECT.md` is
filled in. Output is a versioned machine-facing API: JSON on stdout by
default, optional `-o human` mode, logs on stderr, fixed exit-code taxonomy,
structured error envelope, deterministic ordering, golden fixtures.

---

## 2. Durable user constraints (honour in every turn)

1. **Library decisions in `CLAUDE.md` are not up for debate.** Everything
   else — code, CLAUDE.md prose outside library choices, PROJECT.md,
   README — is in scope.
2. **Look forwards, not backwards.** No `CHANGELOG.md`, no deprecation
   cycles, no migration guides, no legacy carve-outs. If a change would
   need a CHANGELOG entry, reconsider the change. Reinforces the existing
   "no backwards-compatibility shims" stance in CLAUDE.md.
3. **Paramount: questions are questions.** When the user's message
   contains a question mark or interrogative, answer the question
   **literally**. Do not convert interrogative syntax to imperative intent
   via tone. This rule is absolute; the prior session violated it
   multiple times (that's why this file exists instead of committed work).
4. **Do not save auto-memories unless the user asks.** No proactive
   memory writes.
5. **Engage with substance** when criticised — never retreat into terse
   or minimal replies. But also: do not grovel or promise to do better.
6. **State assumptions inline before acting** on anything beyond a
   direct instruction. Do not infer a start from complaints about time.

---

## 3. Repo state on disk

**Not a git repo** (no `.git/`). No commits.

No uncommitted code edits. A brief unauthorized edit to
`internal/output/errors.go` (added `ErrCodeInvalidOutputMode` and
`ErrCodeSubcommandRequired` constants) was reverted. Those constants
still need to be added as part of fixes **B3** and **B4** in §5.1.

The only change from session start is this `HANDOFF.md` file itself —
delete it once acted on.

---

## 4. Open branchpoint — needs user decision

**Env → koanf key mapping convention.** `PROJECT.md`'s config schema has
nested keys (`tls.insecure`); env vars support only `_`. Three options:

- **(a)** `_` → `-`. Flat only. `GO_CLI_TEMPLATE_TLS_INSECURE` →
  `tls-insecure`. Nested env deferred to per-repo.
- **(b)** `__` → `.`, single `_` → `-`. Nested via double-underscore
  convention. `GO_CLI_TEMPLATE_TLS__INSECURE` → `tls.insecure`.
- **(c)** Flatten the whole schema — no nesting anywhere, YAML included.

Prior session recommended **(a)**. Not confirmed. This blocks finding
#2 below; do not land that fix until the user confirms.

No other branchpoints identified — the rest can be landed with sensible
defaults.

---

## 5. Findings

File:line references are against the repo at session start.

### 5.1 Contract bugs (template fails its own CLAUDE.md)

**B1. Env → flag precedence broken.** `GO_CLI_TEMPLATE_OUTPUT=human` does
not switch mode. Env values land in koanf but commands read
`deps.Flags.Output` (pflag-populated struct) directly; merged koanf is
only read by `config view`. Fix: in `internal/cli/root.go` `buildDeps`,
run `loadConfig` first, then backfill `*Flags` fields from
`layers.Merged` when `pfs.Changed(flagName) == false`, then validate and
use. Covers `Output`, `LogLevel`, `Timeout`, `Yes`, `DryRun`. `Config`
flag itself must not be backfilled — chicken-and-egg.
`internal/cli/root.go:160–201`.

**B2. Env key transform mismatches posflag keys.**
`internal/cli/root.go:290` maps `GO_CLI_TEMPLATE_LOG_LEVEL` →
`log.level`. Posflag registers the flag name verbatim (`log-level`).
Keys don't align. Fix depends on branchpoint in §4.

**B3. Help/group output on stdout, exit 0.**
`go-cli-template --help` and bare `go-cli-template config` dump cobra
help to stdout and exit 0. Violates stdout=JSON contract. Fix:
- `cmd.SetOut(os.Stderr)` in `NewRoot` (routes all cobra help/usage to
  stderr).
- Introduce `newGroupCommand(use, short)` helper returning a cobra
  command whose `RunE` returns `*output.Error` with
  `ErrCodeSubcommandRequired` and `ExitUserError`.
- Apply to the `config` group (`internal/cli/config.go`) and the root
  command itself (bare `go-cli-template` must also error). The `help`
  flag path still works because cobra handles `--help` before `RunE`.
`internal/cli/root.go:112–137`, `internal/cli/config.go:8–15`.

**B4. `--output xml` silently accepted.** No validation; both
`output.WriteJSON` and `output.WriteError` switch with `default:` →
JSON. Fix: after backfill in `buildDeps`, validate
`f.Output ∈ {"json", "human"}`; return `*output.Error` with
`ErrCodeInvalidOutputMode`.

**B5. `config view` annotation lies.** `internal/cli/config_view.go`
omits `annotationMachineOnly`; `commands` introspection reports
`human_output: true`; `RunE` rejects `-o human` with
`HUMAN_OUTPUT_NOT_SUPPORTED`. Fix: add `annotationMachineOnly: "true"`.

**B6. Retry contract unimplemented.** `internal/cli/root.go:177–182`:
- No ±20% jitter.
- `Retry-After` header not honoured (and no 10s cap / fail-fast).
- Root context not wrapped in `context.WithTimeout(f.Timeout)`;
  `--timeout` only applies per-request, so the total retry window
  CLAUDE.md specifies is not enforced. Wrap in `PersistentPreRunE`.
- `--no-retry` flag missing.

**B7. Cobra native errors mapped to `UNKNOWN`.** `unknown flag` /
`unknown command` errors pass through `writeErrorAndExit` as plain
errors and are tagged `UNKNOWN`. Fix: in `writeErrorAndExit`
(`internal/cli/root.go:342–354`) detect message prefixes and map to
`INVALID_FLAG` (already exists) / new `UNKNOWN_COMMAND` code.

### 5.2 Contract gaps (CLAUDE.md specifies, template doesn't implement)

**G1. Mutation guard.** CLAUDE.md: mutating commands require `--yes` or
`--dry-run`. Template has no `annotationMutating` and no central
enforcement. Add annotation + `PersistentPreRunE` check that mirrors
the machine-only pattern.

**G2. Secret redaction.** `config view` dumps token values verbatim
(reproducer: set `GO_CLI_TEMPLATE_TOKEN=x`, run `config view`).
`EnableGenerateCurlOnDebug` logs `Authorization:` headers at debug
level. Add: (a) default sensitive-key set (`token`, `password`,
`secret`, keys ending `-token`/`-secret`/`-password`/`-key`),
(b) redaction in `config view.Values`, (c) redaction of secret headers
in debug curl log output. Per-repo extension hook in the sensitive-key
set.

**G3. `internal/testutil/` missing.** CLAUDE.md names it for a small
method+path dispatcher. Directory doesn't exist.

**G4. Taskfile `generate` contradicts its own rule.** CLAUDE.md:
"included only when PROJECT.md declares a codegen or mock opt-in".
Template ships with it. Either delete or move below
`# --- per-repo tasks below ---` marker.

**G5. NDJSON streaming mentioned, not implemented.** No helper, no
example, no rule for buffer-vs-stream. Either scaffold or remove from
CLAUDE.md.

### 5.3 Design issues likely to bite descendants

**D1. `deps.Flags.*` vs `deps.Config.Merged.String(...)` asymmetry.**
Core flags and target config read through different paths. After B1
(backfill), these unify.

**D2. `--config` flag appears in `config view.Values`.** Duplicates
`ConfigPath`. Filter CLI-only flags (`--output`, `--log-level`,
`--config`, `--yes`, `--dry-run`) out of the Values map;
`config view` should show config, not pflag state.

**D3. `errorCodeList()` not sorted.**
`internal/cli/commands.go:134–143` — alphabetise.

**D4. `VersionOutput.Target *string`.**
`internal/cli/version.go:17–20` — `string` with `,omitempty` is
idiomatic; CLAUDE.md "zero values are useful" applies.

**D5. `depsFromContext` panics.**
`internal/cli/root.go:152–158` — emits Go stack trace instead of
structured envelope. Consider returning `*output.Error` with
`UNKNOWN` / exit 1.

### 5.4 PROJECT.md / README / docs

**P1. PROJECT.md mentions `--insecure`,** `internal/cli/root.go` does
not wire it. Either ship in template (with stderr warning) or remove
the mention. Lean: ship it — TLS-off is common enough to warrant a
uniform contract across descendants.

**P2. PROJECT.md config-precedence wording divergent from CLAUDE.md**
("config file" vs "file"). Unify.

**P3. README step 3 underspecifies module path.** `github.com/example/`
prefix must change; only `go-cli-template` is called out.

**P4. README step 4 doesn't name `cliName` const** in
`internal/cli/root.go` — only `envVarPrefix` is mentioned, but both
drive behaviour.

**P5. README doesn't mention deleting or relocating the Taskfile
`generate` task** when codegen is not opted in.

### 5.5 CLAUDE.md self-consistency edits

Items CLAUDE.md should cover but doesn't:

**C1. HTTP retry section** specifies four features (jitter,
`Retry-After` cap, context deadline, `--no-retry`) that the template
doesn't implement. Tighten template (preferred) or soften spec.

**C2. Mutation-guard convention** — add a short subsection under
Audience.

**C3. Secret handling** — add a named section: default sensitive key
set, redaction sites (config view, error details, debug curl), per-repo
extension.

**C4. Help/usage routing** — one sentence: "Cobra help and usage are
routed to stderr; stdout is reserved for command output."

**C5. `--output` value validation** — "unrecognised value exits 1 with
`INVALID_OUTPUT_MODE`."

**C6. `CommandsOutput` as a contract surface** — name it as a versioned
output; skills depend on `exit_codes`, `error_codes`, `path`,
`human_output`, `idempotent`.

---

## 6. Priority order

(CHANGELOG-adjacent items already dropped per constraint #2.)

1. **Env/flag precedence + key transform** — blocked on §4 branchpoint.
   Covers B1, B2, D1.
2. **Stdout discipline** — B3 + C4. `SetOut(os.Stderr)`, root + groups
   error with `SUBCOMMAND_REQUIRED`, introduce `newGroupCommand` helper.
3. **`--output` validation** — B4 + C5.
4. **`config view` annotation** — B5.
5. **Mutation guard** — G1 + C2.
6. **Secret redaction** — G2 + C3.
7. **Retry contract** — B6 + C1.
8. **Cobra-native error mapping** — B7.
9. **README / PROJECT.md / CLAUDE.md self-consistency** — P1–P5, C6.
10. **`internal/testutil/` stub; `generate` task relocation; D2–D5** —
    housekeeping.

---

## 7. Suggested first move for the resuming session

Ask the user to confirm option (a), (b), or (c) from §4 **as a question**
(not a start-signal). If the user says "proceed" without answering, ask
again — do not infer. Work cannot meaningfully begin at priority 1 until
that is decided. Priorities 2–4 can proceed in parallel without it if
the user directs.
