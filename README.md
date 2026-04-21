# go-cli-template

Template repository for Go CLIs that wrap REST target systems. The primary
consumer of any CLI built from this template is a Claude skill.

The full operating contract lives in [`CLAUDE.md`](CLAUDE.md). Per-repo
target facts live in [`PROJECT.md`](PROJECT.md).

## Fork for a new target

1. Click **Use this template** on GitHub.
2. Clone the new repo locally.
3. Rename the module path. The template ships as
   `github.com/example/go-cli-template`; change both halves — the host
   segment (`github.com/example/`) and the CLI name
   (`go-cli-template`) — to your chosen module path
   (e.g. `github.com/<owner>/<cli-name>`) in:
   - `go.mod` (the `module` statement)
   - every Go import (rewrite in bulk with `gofmt -r` or your editor)
   - `cmd/go-cli-template/` (rename the directory to match the new CLI name)
   - `Taskfile.yml` (the `CLI_NAME` variable)
   - `.goreleaser.yaml` (the `project_name` field)
4. Update the identity constants in `internal/cli/root.go`: `cliName`
   (lowercase-kebab; drives help strings, User-Agent, XDG config path,
   cobra annotation keys) and `envVarPrefix` (`GO_CLI_TEMPLATE_`; drives
   env-var prefix and auto-discovery path).
5. Rename `internal/target/` to match the target system (e.g.
   `internal/traefik/`). Update the import in `internal/cli/root.go`
   accordingly.
6. Fill in `PROJECT.md` per its embedded placeholders.
7. Choose and install a licence in place of the stub `LICENSE`.
8. Run `task verify`. The skeleton must pass out of the box.
9. Write the first target-system command under `internal/cli/` and wire it
   into `internal/cli/root.go`. Add a golden fixture in `testdata/` per the
   convention in `testdata/README.md`.
10. If you opt into codegen (per `CLAUDE.md` "API client generation") or
    mock generation (per `CLAUDE.md` "Mocking / test doubles"), add a
    `generate` task to `Taskfile.yml` — the template does not ship one
    by default.
