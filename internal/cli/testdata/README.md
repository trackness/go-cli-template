# internal/cli/testdata

Golden fixtures for the `cli` package tests. Convention document is
at the repo-root `testdata/README.md`; this directory holds the
actual JSON bytes.

Regenerate after an intentional output-shape change:

    go test ./internal/cli/... -update

Captured by `internal/testutil.AssertJSONGolden`, which re-indents
through `json.Indent` so diffs are line-oriented. The template's
production `output.WriteJSON` emits compact JSON; indentation lives
only in these fixtures where humans read diffs.
