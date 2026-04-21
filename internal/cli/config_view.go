package cli

import (
	"sort"

	"github.com/spf13/cobra"

	"github.com/example/go-cli-template/internal/output"
)

// ConfigViewOutput is the JSON shape of `config view`.
type ConfigViewOutput struct {
	ConfigPath       string                 `json:"config_path"`
	ConfigPathSource string                 `json:"config_path_source"`
	Values           map[string]ConfigValue `json:"values"`
}

// ConfigValue is one resolved config key's value and the source it came
// from. Source is one of: "flag", "env", "file:<path>", "default".
type ConfigValue struct {
	Value  any    `json:"value"`
	Source string `json:"source"`
}

func newConfigViewCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "view",
		Short: "Emit resolved config with per-value source attribution",
		Annotations: map[string]string{
			annotationMachineOnly: "true",
			annotationIdempotent:  "true",
		},
		RunE: func(c *cobra.Command, _ []string) error {
			deps := depsFromContext(c.Context())
			if deps.Flags.Output == "human" {
				return &output.Error{
					Code:     output.ErrCodeHumanOutputNotSupported,
					Message:  "config view has no human rendering in the template; per-repo may add one",
					ExitCode: output.ExitUserError,
				}
			}
			return output.WriteJSON(deps.Stdout, buildConfigView(deps.Config, deps.Flags.configSource))
		},
	}
}

func buildConfigView(layers *ConfigLayers, pathSource string) ConfigViewOutput {
	out := ConfigViewOutput{
		ConfigPath:       layers.FilePath,
		ConfigPathSource: pathSource,
		Values:           map[string]ConfigValue{},
	}
	keys := layers.Merged.Keys()
	sort.Strings(keys)
	for _, k := range keys {
		out.Values[k] = ConfigValue{
			Value:  output.RedactValue(k, layers.Merged.Get(k)),
			Source: attributeSource(layers, k),
		}
	}
	return out
}

// attributeSource walks the layers highest-precedence-first and returns
// the source the key came from.
func attributeSource(layers *ConfigLayers, key string) string {
	switch {
	case layers.Flags.Exists(key):
		return "flag"
	case layers.Env.Exists(key):
		return "env"
	case layers.File.Exists(key):
		if layers.FilePath != "" {
			return "file:" + layers.FilePath
		}
		return "file"
	default:
		return "default"
	}
}
