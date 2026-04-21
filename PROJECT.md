# PROJECT.md

Per-repo context for this CLI. Read by Claude Code via `@./PROJECT.md` from
`CLAUDE.md`. Everything specific to *this* CLI — what it wraps, its domain
vocabulary, auth, endpoints, retry policy — lives here. Repo-specific
variance belongs here, not in `CLAUDE.md`.

## What this CLI does

<one or two sentences — what it wraps and who calls it>

## Target system

- **Name and version:** <e.g. Traefik v3.x>
- **Primary documentation:** <URL>
- **API style:** <REST / gRPC / file-based / mixed>
- **Stability assumption:** <which versions of the target this CLI supports>

## Authentication

- **Mechanism:** <bearer token / mTLS / basic / none>
- **Env var:** `<CLI_NAME>_TOKEN` (or equivalent)
- **Flag:** `--token` (overrides env)
- **Config precedence:** flag > env > file > default

## Endpoints and connection

- **Flag:** `--endpoint` (default `<URL>`)
- **Env var:** `<CLI_NAME>_ENDPOINT`
- **Default timeout:** <Ns>

## Retry policy

Defaults are pinned in `CLAUDE.md` (GET/HEAD only, 429/502/503/504,
3 attempts, exponential backoff with ±20% jitter, 10s `Retry-After` cap).
Override here only when the target's documented contract justifies it.

- **Auto-retried methods:** <list — e.g. GET, HEAD. Add PUT/DELETE only
  with a citation to target documentation establishing idempotency for the
  specific endpoints in scope.>
- **Retryable status codes:** <default set unless target behaviour differs>
- **`Retry-After` cap:** <default 10s unless target commonly asks for more
  and the CLI reasonably can wait>
- **Deviations from default:** <cite documentation; no deviation
  without a stated reason>

When in doubt, leave the default in place and let the skill layer
decide.

## Domain vocabulary

Terms from the target system used in this CLI, so Claude uses them correctly:

- **<Term>:** <brief definition, link to source doc>
- **<Term>:** <...>

## Command tree (high level)

List the command groups this CLI exposes. Detail lives in `--help` and the
`commands` subcommand output.

- `<group>` — <purpose>
- `<group>` — <purpose>

## Output shapes specific to this CLI

If there are resource types with stable JSON shapes skills will depend on,
name them here and link to the relevant Go types in `internal/output/`.

- `<Resource>` → `internal/output/<file>.go`

## API client generation

Default is hand-written. Per `CLAUDE.md`, type-only codegen is
permitted only when all three criteria are documented here.

- **Approach:** <hand-written | type-only codegen via oapi-codegen/v2>

If codegen:

- **Availability:** official spec at <URL>, vendored at
  `internal/<target>/gen/spec.yaml` (version/tag: <version>)
- **Quality:** validated clean on <list of endpoints>; spot-check date:
  <date>; endpoints compared live: <3 endpoints>; discrepancies:
  <none | list>
- **Scope payoff:** estimated <N> lines of hand-written type definitions
  avoided (must exceed ~500)
- **Last regenerated:** <date>, spec version at that date: <version>

If hand-written: no justification required; this is the default.

## Mocking / test doubles

Default is hand-rolled. Per `CLAUDE.md`, `go.uber.org/mock` is
permitted per-repo for specific interfaces meeting both criteria.

- **Approach:** <hand-rolled | hybrid (some interfaces generated)>

If hybrid, list each interface covered:

- **Interface:** `<pkg>.<Name>`
  - Method count: <N> (must be 10 or more)
  - Test files using it: <list> (must be 3 or more)
  - Why hand-rolling would be materially worse: <specifics>

All other interfaces remain hand-rolled.

## Config file schema

Keys the YAML config file supports (resolved per `CLAUDE.md` precedence:
flag > env > file > default). Each key maps 1:1 to a persistent flag and an
env var of form `<CLI_NAME>_<KEY>`.

```yaml
endpoint: <url>
token: <string>           # sensitive; prefer env or flag over file
timeout: <duration>       # e.g. 30s
log-level: info
```

Required keys at runtime: `endpoint`, `token`. Missing either exits `1` with
a structured error.

## Known target-system quirks

Anything that would otherwise confuse Claude when reading error responses or
unusual status codes from the target.

- <quirk>
- <quirk>
