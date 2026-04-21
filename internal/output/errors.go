// Package output owns the skill-facing output contract for the CLI.
//
// See CLAUDE.md, "Audience and output contract", for the rules this
// package enforces: the stable exit-code taxonomy, the structured error
// envelope, strict JSON default mode, and deterministic optional human
// mode.
//
// Commands render successful output via WriteJSON (or the human helpers
// in human.go / colour.go when the command opts in to -o human) and
// return *Error values on failure. The root command's error handler
// renders the envelope and maps to the exit code.
package output

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// ExitCode is the CLI's stable exit-code taxonomy. Skills hard-code
// branches on these values; never repurpose a number.
type ExitCode int

// Exit-code constants. Frozen per the output contract; changing a value
// is a major-version bump.
const (
	ExitSuccess        ExitCode = 0
	ExitUserError      ExitCode = 1
	ExitTargetError    ExitCode = 2
	ExitTransportError ExitCode = 3
	ExitInterrupted    ExitCode = 130
)

// Error code constants in UPPER_SNAKE_CASE per the contract. Adding a
// new code is additive; renaming or removing is a major-version bump.
const (
	ErrCodeConfirmationRequired    = "CONFIRMATION_REQUIRED"
	ErrCodeHumanOutputNotSupported = "HUMAN_OUTPUT_NOT_SUPPORTED"
	ErrCodeInvalidFlag             = "INVALID_FLAG"
	ErrCodeInvalidLogLevel         = "INVALID_LOG_LEVEL"
	ErrCodeInvalidOutputMode       = "INVALID_OUTPUT_MODE"
	ErrCodeMalformedConfigFile     = "MALFORMED_CONFIG_FILE"
	ErrCodeMissingRequiredValue    = "MISSING_REQUIRED_VALUE"
	ErrCodeSubcommandRequired      = "SUBCOMMAND_REQUIRED"
	ErrCodeUnknown                 = "UNKNOWN"
	ErrCodeUnknownCommand          = "UNKNOWN_COMMAND"
)

// Envelope is the JSON shape of an error written to stderr in default
// mode:
//
//	{"error": {"code": "...", "message": "...", "details": {...}}}
type Envelope struct {
	Error Body `json:"error"`
}

// Body is the nested error body. Details is omitted from the JSON when
// empty.
type Body struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

// Error is the structured error returned by commands. Retrieve from a
// wrapped chain with errors.As(err, &oe).
type Error struct {
	Code     string
	Message  string
	Details  map[string]any
	ExitCode ExitCode
	Cause    error
}

// Error implements the error interface.
func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap supports errors.Is / errors.As.
func (e *Error) Unwrap() error { return e.Cause }

// Envelope returns the wire-shape envelope for this error.
func (e *Error) Envelope() Envelope {
	return Envelope{Error: Body{
		Code:    e.Code,
		Message: e.Message,
		Details: e.Details,
	}}
}

// WriteError renders err to w in the given output mode. In default
// (JSON) mode the envelope is written as a single JSON document. In
// "human" mode a plain "Error: <message>" line is written. Callers pass
// os.Stderr in both cases; logs and errors share stderr.
func WriteError(w io.Writer, mode string, err error) {
	var oe *Error
	if !errors.As(err, &oe) {
		oe = &Error{
			Code:     ErrCodeUnknown,
			Message:  err.Error(),
			ExitCode: ExitUserError,
		}
	}
	switch mode {
	case "human":
		_, _ = fmt.Fprintln(w, "Error: "+oe.Message)
	default:
		enc := json.NewEncoder(w)
		enc.SetEscapeHTML(false)
		_ = enc.Encode(oe.Envelope())
	}
}
