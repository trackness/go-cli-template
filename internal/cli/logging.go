package cli

import (
	"fmt"
	"log/slog"

	"github.com/example/go-cli-template/internal/output"
)

// slogRestyLogger adapts *slog.Logger to resty's Logger interface and
// redacts sensitive HTTP headers from debug-level curl dumps before
// they reach stderr. Installed on the resty client in buildDeps so
// every debug-mode request goes through the redaction pipeline.
type slogRestyLogger struct {
	*slog.Logger
}

func (l *slogRestyLogger) Errorf(format string, v ...any) {
	l.Error(output.RedactCurl(fmt.Sprintf(format, v...)))
}

func (l *slogRestyLogger) Warnf(format string, v ...any) {
	l.Warn(output.RedactCurl(fmt.Sprintf(format, v...)))
}

func (l *slogRestyLogger) Debugf(format string, v ...any) {
	l.Debug(output.RedactCurl(fmt.Sprintf(format, v...)))
}
