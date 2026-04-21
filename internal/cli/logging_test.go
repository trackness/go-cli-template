package cli

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestSlogRestyLogger_DebugfRedactsCurl(t *testing.T) {
	var buf bytes.Buffer
	base := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	adapter := &slogRestyLogger{Logger: base}
	adapter.Debugf("curl -X GET 'http://x' -H 'Authorization: Bearer abc123secret'")

	got := buf.String()
	if strings.Contains(got, "abc123secret") {
		t.Errorf("secret leaked into slog output: %q", got)
	}
	if !strings.Contains(got, "<redacted>") {
		t.Errorf("redacted placeholder missing: %q", got)
	}
}

func TestSlogRestyLogger_ErrorfAndWarnf_PassThrough(t *testing.T) {
	var buf bytes.Buffer
	base := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	adapter := &slogRestyLogger{Logger: base}

	adapter.Errorf("transport failed: %s", "connection refused")
	adapter.Warnf("retry %d of %d", 2, 3)

	got := buf.String()
	if !strings.Contains(got, "transport failed: connection refused") {
		t.Errorf("Errorf lost formatting: %q", got)
	}
	if !strings.Contains(got, "retry 2 of 3") {
		t.Errorf("Warnf lost formatting: %q", got)
	}
}
