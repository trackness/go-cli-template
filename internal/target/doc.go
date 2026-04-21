// Package target is a placeholder for the target-system client. Rename the
// directory and package to match the target when forking (e.g. traefik,
// cloudflare, vault).
//
// What belongs here:
//
//   - client.go wrapping the *resty.Client constructed in
//     internal/cli/root.go with domain-specific request methods;
//   - per-resource Go types;
//   - small, focused interfaces at the command boundary (not
//     *resty.Client directly) so unit tests can fake them.
//
// See CLAUDE.md, sections "HTTP client", "HTTP retry", and
// "API client generation", for the rules that govern this package.
package target
