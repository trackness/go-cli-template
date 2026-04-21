package output

import (
	"io"

	"github.com/jedib0t/go-pretty/v6/table"
)

// NewTable returns a go-pretty table writer configured for the
// template's default human style (StyleLight). Callers build the table
// (AppendHeader, AppendRow, ...) and call Render to emit to w.
//
// Plain aligned-column output may instead use stdlib text/tabwriter per
// CLAUDE.md ("Human output rendering").
func NewTable(w io.Writer) table.Writer {
	t := table.NewWriter()
	t.SetOutputMirror(w)
	t.SetStyle(table.StyleLight)
	return t
}
