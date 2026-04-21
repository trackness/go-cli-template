# go-cli-template

Template repository for Go CLIs that wrap REST target systems. The primary
consumer of any CLI built from this template is a Claude skill.

The full operating contract lives in [`CLAUDE.md`](CLAUDE.md). Per-repo
target facts live in [`PROJECT.md`](PROJECT.md).

## Fork for a new target

1. Click **Use this template** on GitHub.
2. Clone the new repo locally.
3. Replace `go-cli-template` (lowercase, kebab) with the new CLI name in:
   - `go.mod` (module path)
   - `cmd/go-cli-template/` (rename the directory)
   - `Taskfile.yml` (the `CLI_NAME` variable)
   - `.goreleaser.yaml` (the `project_name` field)
   - every Go import
4. Replace `GO_CLI_TEMPLATE` (upper, snake) with the uppercase form in
   `internal/cli/root.go` (the `envVarPrefix` constant, which drives the
   config env-var prefix and auto-discovery path).
5. Rename `internal/target/` to match the target system (e.g.
   `internal/traefik/`). Update the import in `internal/cli/root.go`
   accordingly.
6. Fill in `PROJECT.md` per its embedded placeholders.
7. Choose and install a licence in place of the stub `LICENSE`.
8. Run `task verify`. The skeleton must pass out of the box.
9. Write the first target-system command under `internal/cli/` and wire it
   into `internal/cli/root.go`. Add a golden fixture in `testdata/` per the
   convention in `testdata/README.md`.
