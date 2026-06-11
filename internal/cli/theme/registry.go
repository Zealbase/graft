package theme

import (
	"os"
	"sync/atomic"

	"github.com/mattn/go-isatty"
)

var (
	active  atomic.Pointer[Theme]
	noColor atomic.Bool
)

func init() {
	active.Store(Dark())
}

// Activate sets the active theme by name and disables colour when stdout is not
// a TTY (piped, redirected, CI). Unknown names fall back to "dark".
func Activate(name string) {
	fd := os.Stdout.Fd()
	if !isatty.IsTerminal(fd) && !isatty.IsCygwinTerminal(fd) {
		noColor.Store(true)
	}
	active.Store(build(name))
}

// SetNoColor forces colour suppression on or off (test seam / explicit flag).
func SetNoColor(v bool) { noColor.Store(v) }

// NoColor reports whether colour output is suppressed.
func NoColor() bool { return noColor.Load() }

// Active returns the currently active theme.
func Active() *Theme { return active.Load() }

// Names lists the built-in theme names.
func Names() []string {
	return []string{"dark", "dark-dim", "light", "colorblind"}
}

// IsvalidName reports whether name is a known built-in theme.
func IsValidName(name string) bool {
	for _, n := range Names() {
		if n == name {
			return true
		}
	}
	return false
}

func build(name string) *Theme {
	switch name {
	case "dark-dim":
		return DarkDim()
	case "light":
		return Light()
	case "colorblind":
		return Colorblind()
	default:
		return Dark()
	}
}
