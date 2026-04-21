package output

import (
	"os"

	"golang.org/x/term"
)

// ANSI escape sequences used by human mode. Stdlib-only; fatih/color is
// adopted only when the threshold in CLAUDE.md fires.
const (
	ansiReset  = "\x1b[0m"
	ansiRed    = "\x1b[31m"
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
	ansiDim    = "\x1b[2m"
)

// IsTerminal reports whether fd is an interactive terminal.
func IsTerminal(fd uintptr) bool { return term.IsTerminal(int(fd)) }

// StdoutIsTerminal is a convenience wrapper around IsTerminal(stdout).
// Use it in human mode to decide whether colour output is permitted.
func StdoutIsTerminal() bool { return IsTerminal(os.Stdout.Fd()) }

// Paint wraps s in the given ANSI escape when active is true; otherwise
// returns s unchanged. Typical callers pass active = (mode == "human" &&
// StdoutIsTerminal()).
func Paint(active bool, code, s string) string {
	if !active || code == "" {
		return s
	}
	return code + s + ansiReset
}

// Red wraps s in ANSI red when active is true.
func Red(active bool, s string) string { return Paint(active, ansiRed, s) }

// Green wraps s in ANSI green when active is true.
func Green(active bool, s string) string { return Paint(active, ansiGreen, s) }

// Yellow wraps s in ANSI yellow when active is true.
func Yellow(active bool, s string) string { return Paint(active, ansiYellow, s) }

// Dim wraps s in ANSI dim when active is true.
func Dim(active bool, s string) string { return Paint(active, ansiDim, s) }
