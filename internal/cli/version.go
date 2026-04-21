package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/example/go-cli-template/internal/output"
)

// VersionOutput is the JSON shape of the `version` command. Skills
// parse this to learn both the CLI version and — once a target call
// has been made during this invocation — the target system's reported
// version. The template does not populate Target; per-repo code that
// wraps a real target should populate it on its first successful call.
// Empty strings are omitted from the JSON; no need for a pointer.
type VersionOutput struct {
	CLI    BuildInfo `json:"cli"`
	Target string    `json:"target,omitempty"`
}

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Emit the CLI version",
		Annotations: map[string]string{
			annotationIdempotent: "true",
		},
		RunE: func(c *cobra.Command, _ []string) error {
			deps := depsFromContext(c.Context())
			out := VersionOutput{CLI: deps.Build}

			if deps.Flags.Output == "human" {
				line := fmt.Sprintf("%s %s", cliName, deps.Build.Version)
				if deps.Build.Commit != "" || deps.Build.Date != "" {
					line += fmt.Sprintf(" (%s %s)", deps.Build.Commit, deps.Build.Date)
				}
				_, _ = fmt.Fprintln(deps.Stdout, line)
				return nil
			}
			return output.WriteJSON(deps.Stdout, out)
		},
	}
}
