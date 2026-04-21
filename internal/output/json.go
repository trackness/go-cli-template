package output

import (
	"encoding/json"
	"io"
)

// WriteJSON writes v as a single JSON document to w, terminated by a
// newline. Strict mode: no banners, no ANSI, no trailing prose. NDJSON
// streams are produced by calling WriteJSON on successive values.
//
// HTML-escaping is disabled because the consumer is machine-facing and
// must receive `<`, `>`, and `&` unmangled.
func WriteJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}
