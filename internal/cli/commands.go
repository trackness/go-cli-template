package cli

import (
	"sort"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/example/go-cli-template/internal/output"
)

// CommandsOutput is the JSON shape of `commands` introspection. It is
// the skill author's primary artifact: pasted into a SKILL.md scaffold
// it describes the full CLI surface the agent needs to invoke the
// tool. Golden-tested in `testdata/` — skills hard-code field names.
type CommandsOutput struct {
	Name       string         `json:"name"`
	Commands   []CommandEntry `json:"commands"`
	ExitCodes  map[string]int `json:"exit_codes"`
	ErrorCodes []string       `json:"error_codes"`
}

// CommandEntry describes a single command in the tree. Path is the
// sequence from root to this command, inclusive; the root has [].
type CommandEntry struct {
	Path        []string    `json:"path"`
	Short       string      `json:"short"`
	Flags       []FlagEntry `json:"flags"`
	HumanOutput bool        `json:"human_output"`
	Idempotent  bool        `json:"idempotent"`
}

// FlagEntry describes one flag declared on this command (local or
// persistent). Inherited persistent flags appear on their declaring
// command only, not on every descendant.
type FlagEntry struct {
	Name        string `json:"name"`
	Shorthand   string `json:"shorthand,omitempty"`
	Type        string `json:"type"`
	Default     string `json:"default"`
	Description string `json:"description"`
	Persistent  bool   `json:"persistent"`
}

func newCommandsCommand(root *cobra.Command) *cobra.Command {
	return &cobra.Command{
		Use:   "commands",
		Short: "Emit the full command tree as JSON (introspection for skills)",
		Annotations: map[string]string{
			annotationMachineOnly: "true",
			annotationIdempotent:  "true",
		},
		RunE: func(c *cobra.Command, _ []string) error {
			deps := depsFromContext(c.Context())
			if deps.Flags.Output == "human" {
				return &output.Error{
					Code:     output.ErrCodeHumanOutputNotSupported,
					Message:  "commands is machine-only; do not pass -o human",
					ExitCode: output.ExitUserError,
				}
			}
			return output.WriteJSON(deps.Stdout, walkCommands(root))
		},
	}
}

func walkCommands(root *cobra.Command) CommandsOutput {
	var entries []CommandEntry
	var visit func(cmd *cobra.Command, path []string)
	visit = func(cmd *cobra.Command, path []string) {
		if cmd.Name() == "help" || cmd.Hidden {
			return
		}
		entries = append(entries, CommandEntry{
			Path:  path,
			Short: cmd.Short,
			Flags: collectFlags(cmd),
			// Groups (commands with subcommands) never render human
			// output — they dispatch or error with SUBCOMMAND_REQUIRED.
			// Leaves render unless explicitly annotated machine-only.
			HumanOutput: !cmd.HasSubCommands() && cmd.Annotations[annotationMachineOnly] != "true",
			Idempotent:  cmd.Annotations[annotationIdempotent] == "true",
		})
		children := append([]*cobra.Command(nil), cmd.Commands()...)
		sort.Slice(children, func(i, j int) bool {
			return children[i].Name() < children[j].Name()
		})
		for _, ch := range children {
			visit(ch, append(append([]string{}, path...), ch.Name()))
		}
	}
	visit(root, []string{})

	return CommandsOutput{
		Name:       root.Name(),
		Commands:   entries,
		ExitCodes:  exitCodeMap(),
		ErrorCodes: errorCodeList(),
	}
}

func collectFlags(cmd *cobra.Command) []FlagEntry {
	entries := []FlagEntry{}
	// LocalFlags = flags declared on this command (both local and
	// persistent), excluding inherited persistent flags from ancestors.
	// `help` is emitted by cobra on every command; it is universal and
	// has no skill-author value — omitted.
	cmd.LocalFlags().VisitAll(func(f *pflag.Flag) {
		if f.Name == "help" {
			return
		}
		persistent := cmd.PersistentFlags().Lookup(f.Name) != nil
		entries = append(entries, FlagEntry{
			Name:        f.Name,
			Shorthand:   f.Shorthand,
			Type:        f.Value.Type(),
			Default:     f.DefValue,
			Description: f.Usage,
			Persistent:  persistent,
		})
	})
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return entries
}

func exitCodeMap() map[string]int {
	return map[string]int{
		"SUCCESS":         int(output.ExitSuccess),
		"USER_ERROR":      int(output.ExitUserError),
		"TARGET_ERROR":    int(output.ExitTargetError),
		"TRANSPORT_ERROR": int(output.ExitTransportError),
		"INTERRUPTED":     int(output.ExitInterrupted),
	}
}

func errorCodeList() []string {
	return []string{
		output.ErrCodeConfirmationRequired,
		output.ErrCodeHumanOutputNotSupported,
		output.ErrCodeInvalidFlag,
		output.ErrCodeInvalidLogLevel,
		output.ErrCodeInvalidOutputMode,
		output.ErrCodeMalformedConfigFile,
		output.ErrCodeMissingRequiredValue,
		output.ErrCodeRetryAfterTooLong,
		output.ErrCodeSubcommandRequired,
		output.ErrCodeUnknown,
		output.ErrCodeUnknownCommand,
	}
}
