package cli

import "github.com/spf13/cobra"

// newConfigCommand constructs the `config` command group. Subcommands
// (e.g. `view`) live in their own files per the one-file-per-command
// rule in CLAUDE.md.
func newConfigCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Inspect configuration",
	}
	cmd.AddCommand(newConfigViewCommand())
	return cmd
}
