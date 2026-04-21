package cli

import (
	"errors"
	"testing"

	"github.com/example/go-cli-template/internal/output"
)

// TestMapCobraNativeError_KnownPrefixes locks the message-prefix
// detection against cobra/pflag prose changes. If a dependency bump
// rewords one of these, the test fails loudly rather than silently
// falling back to ErrCodeUnknown. Messages are taken verbatim from
// cobra/pflag sources as of the currently pinned version.
func TestMapCobraNativeError_KnownPrefixes(t *testing.T) {
	tests := []struct {
		name     string
		msg      string
		wantCode string
	}{
		{"unknown long flag", "unknown flag: --foo", output.ErrCodeInvalidFlag},
		{"unknown shorthand flag", "unknown shorthand flag: 'F' in -F", output.ErrCodeInvalidFlag},
		{"flag not defined", "flag provided but not defined: --foo", output.ErrCodeInvalidFlag},
		{"unknown command", `unknown command "foo" for "bar"`, output.ErrCodeUnknownCommand},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapCobraNativeError(errors.New(tt.msg))
			if got == nil {
				t.Fatalf("mapCobraNativeError(%q) = nil, want code %q", tt.msg, tt.wantCode)
			}
			if got.Code != tt.wantCode {
				t.Errorf("code = %q, want %q", got.Code, tt.wantCode)
			}
			if got.ExitCode != output.ExitUserError {
				t.Errorf("exit code = %d, want %d", got.ExitCode, output.ExitUserError)
			}
		})
	}
}

// TestMapCobraNativeError_UnrelatedPassesThrough asserts the mapper
// returns nil for anything that doesn't match a known prefix, so the
// caller can leave the original error untouched.
func TestMapCobraNativeError_UnrelatedPassesThrough(t *testing.T) {
	got := mapCobraNativeError(errors.New("something else entirely"))
	if got != nil {
		t.Errorf("unrelated error mapped unexpectedly: %+v", got)
	}
}
