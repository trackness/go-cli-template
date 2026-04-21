# testdata

Golden fixtures for command output.

## Naming convention

- `testdata/<command>.json` — golden for default (JSON) mode.
- `testdata/<command>.human.txt` — golden for `-o human` mode; omit when
  the command is machine-only.
- `testdata/<command>.<variant>.json` / `.human.txt` — additional variants
  (e.g. `ping.timeout.json` for the timeout-error case).

Canned HTTP responses used by `httptest`-backed integration tests live
alongside their goldens, named `<command>.<variant>.response.<ext>`.

## Regeneration

Fixtures are captured by `internal/testutil.AssertGolden`. Regenerate
after an intentional shape change with:

    go test ./... -update

Do not edit by hand. Per `CLAUDE.md` (Audience and output contract),
any output shape change requires a golden update and bumps the major
version.
