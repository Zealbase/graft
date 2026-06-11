// Package theme maps semantic UI roles to ANSI escape sequences for graft's CLI
// output. Colour is suppressed automatically when stdout is not a TTY (piped,
// redirected, CI) so machine-readable output (-o json|yaml) stays raw.
package theme

const (
	reset = "\033[0m"
	bold  = "\033[1m"
	dim   = "\033[2m"

	fgBlack        = "\033[30m"
	fgRed          = "\033[31m"
	fgGreen        = "\033[32m"
	fgYellow       = "\033[33m"
	fgCyan         = "\033[36m"
	fgWhite        = "\033[37m"
	fgBrightRed    = "\033[91m"
	fgBrightGreen  = "\033[92m"
	fgBrightYellow = "\033[93m"
	fgBrightBlue   = "\033[94m"
	fgBrightWhite  = "\033[97m"

	fgOrange  = "\033[38;5;208m"
	fgNavy    = "\033[38;5;25m"
	fgTeal    = "\033[38;5;37m"
	fgAmber   = "\033[38;5;214m"
	fgPurple  = "\033[38;5;135m"
	fgDimGray = "\033[38;5;243m"
)

// Role identifies a semantic colour slot in the terminal UI.
type Role int

const (
	RoleCommand Role = iota
	RoleFlagName
	RoleFlagValue
	RoleFlagDesc
	RoleRequired
	RoleSectionHead
	RolePositional

	RoleLogDebug
	RoleLogInfo
	RoleLogWarn
	RoleLogError
	RoleLogFatal

	roleSentinel // must stay last
)

// Theme maps roles to ANSI prefixes via a fixed-size array (alloc-free Color()).
type Theme struct {
	Name   string
	colors [roleSentinel]string
}

// Reset is the ANSI reset sequence.
const Reset = reset

// Color returns the ANSI prefix for r, or "" when unset.
func (t *Theme) Color(r Role) string {
	if r < 0 || r >= roleSentinel {
		return ""
	}
	return t.colors[r]
}

// Apply wraps text with the colour for role r and a reset. Returns text
// unchanged when colour output is suppressed or the role has no colour.
func (t *Theme) Apply(r Role, text string) string {
	if noColor.Load() {
		return text
	}
	c := t.Color(r)
	if c == "" {
		return text
	}
	return c + text + reset
}

// Dark is the default dark-background theme.
func Dark() *Theme {
	t := &Theme{Name: "dark"}
	t.colors[RoleCommand] = bold
	t.colors[RoleFlagName] = fgBrightBlue
	t.colors[RoleFlagValue] = fgYellow
	t.colors[RoleFlagDesc] = fgWhite
	t.colors[RoleRequired] = fgBrightRed
	t.colors[RoleSectionHead] = bold + fgBrightWhite
	t.colors[RolePositional] = fgCyan
	t.colors[RoleLogDebug] = dim + fgDimGray
	t.colors[RoleLogInfo] = fgBrightGreen
	t.colors[RoleLogWarn] = fgBrightYellow
	t.colors[RoleLogError] = fgBrightRed
	t.colors[RoleLogFatal] = bold + fgBrightRed
	return t
}

// DarkDim is a low-contrast dark theme.
func DarkDim() *Theme {
	t := &Theme{Name: "dark-dim"}
	t.colors[RoleCommand] = bold
	t.colors[RoleFlagName] = fgCyan
	t.colors[RoleFlagValue] = dim + fgYellow
	t.colors[RoleFlagDesc] = dim + fgWhite
	t.colors[RoleRequired] = fgRed
	t.colors[RoleSectionHead] = bold
	t.colors[RolePositional] = dim + fgCyan
	t.colors[RoleLogDebug] = dim + fgDimGray
	t.colors[RoleLogInfo] = fgGreen
	t.colors[RoleLogWarn] = fgYellow
	t.colors[RoleLogError] = fgRed
	t.colors[RoleLogFatal] = bold + fgRed
	return t
}

// Light is a light-background theme.
func Light() *Theme {
	t := &Theme{Name: "light"}
	t.colors[RoleCommand] = bold
	t.colors[RoleFlagName] = fgNavy
	t.colors[RoleFlagValue] = fgAmber
	t.colors[RoleFlagDesc] = fgBlack
	t.colors[RoleRequired] = fgRed
	t.colors[RoleSectionHead] = bold + fgBlack
	t.colors[RolePositional] = fgTeal
	t.colors[RoleLogDebug] = dim + fgDimGray
	t.colors[RoleLogInfo] = fgGreen
	t.colors[RoleLogWarn] = fgAmber
	t.colors[RoleLogError] = fgRed
	t.colors[RoleLogFatal] = bold + fgRed
	return t
}

// Colorblind uses blue/orange/purple instead of red/green distinctions.
func Colorblind() *Theme {
	t := &Theme{Name: "colorblind"}
	t.colors[RoleCommand] = bold
	t.colors[RoleFlagName] = fgBrightBlue
	t.colors[RoleFlagValue] = fgOrange
	t.colors[RoleFlagDesc] = fgWhite
	t.colors[RoleRequired] = fgOrange
	t.colors[RoleSectionHead] = bold + fgBrightWhite
	t.colors[RolePositional] = fgBrightBlue
	t.colors[RoleLogDebug] = dim + fgDimGray
	t.colors[RoleLogInfo] = fgBrightBlue
	t.colors[RoleLogWarn] = fgOrange
	t.colors[RoleLogError] = fgPurple
	t.colors[RoleLogFatal] = bold + fgPurple
	return t
}
