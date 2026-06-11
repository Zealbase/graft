package cli

import (
	"bytes"
	"io"

	"github.com/Shaik-Sirajuddin/graft/internal/cli/theme"
)

// logWriter colourises leading [LEVEL] tags on log lines using the active theme.
type logWriter struct{ w io.Writer }

// newLogWriter returns a writer that themes log-level tags.
func newLogWriter(w io.Writer) io.Writer { return &logWriter{w: w} }

var levelRoles = map[string]theme.Role{
	"[DEBUG]": theme.RoleLogDebug,
	"[INFO]":  theme.RoleLogInfo,
	"[WARN]":  theme.RoleLogWarn,
	"[ERROR]": theme.RoleLogError,
	"[FATAL]": theme.RoleLogFatal,
}

func (lw *logWriter) Write(p []byte) (int, error) {
	t := theme.Active()
	for tag, role := range levelRoles {
		tagB := []byte(tag)
		if bytes.HasPrefix(p, tagB) {
			out := append([]byte(t.Apply(role, tag)), p[len(tagB):]...)
			_, err := lw.w.Write(out)
			return len(p), err
		}
	}
	return lw.w.Write(p)
}
