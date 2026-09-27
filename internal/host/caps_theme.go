package host

import (
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/abdul-hamid-achik/tuimark/internal/css"
)

// SetTheme makes the tools render with the built-in theme name ("dark" or
// "light") instead of the document's theme attribute (SPEC §26.4): the
// token set that :root tokens then override, and the theme @media
// (theme: …) matches, for Frame, Dump, Validate, and Static alike,
// including the static token check (V003 for a token the theme does not
// define). It is the hook behind the CLI's --theme and the golden
// manifest's theme; it is not public API. The host's @theme still beats
// it (§26.4). "auto" and any other value are errors: no tool probes the
// terminal, so --theme auto stays a usage error in 0.2b (§15.1).
func (a *App) SetTheme(name string) error {
	if name != "dark" && name != "light" {
		return fmt.Errorf("tuimark: unknown theme %q (want dark or light)", name)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.flagTheme = name
	return nil
}

// Theme returns the effective theme of the tools and of Dump and Validate
// (SPEC §26.4): @theme when the host set it, else the one SetTheme chose,
// else the document's theme attribute, else "dark"; auto is dark there.
// It is never "auto", and never a theme Run resolved from the terminal.
func (a *App) Theme() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.toolTheme()
}

// themeChoice is the theme a render starts from before auto is resolved:
// @theme, else --theme, else the document's theme. The caller holds a.mu.
func (a *App) themeChoice() string {
	switch {
	case a.hostTheme != "":
		return a.hostTheme
	case a.flagTheme != "":
		return a.flagTheme
	}
	return a.doc.Theme
}

// toolTheme is the effective theme of Dump, Validate, and the commands
// (SPEC §26.4): auto is dark. The caller holds a.mu.
func (a *App) toolTheme() string {
	return css.EffectiveTheme(a.themeChoice())
}

// frameTheme is the effective theme of a live frame (Frame, which Run and
// the commands share): while Run runs, TUIMARK_THEME beats @theme, which
// beats the document, and auto is what Run resolved from the terminal;
// otherwise it is toolTheme. The caller holds a.mu.
func (a *App) frameTheme() string {
	if a.runEnvTheme != "" {
		return a.runEnvTheme
	}
	if c := a.themeChoice(); c == "auto" && a.runAuto != "" {
		return a.runAuto
	}
	return a.toolTheme()
}

// validateThemes are the themes Validate renders: both, dark first, for a
// theme="auto" document that neither @theme nor --theme overrides (SPEC
// §15.1), else the effective theme. The caller holds a.mu.
func (a *App) validateThemes() []string {
	if a.themeChoice() == "auto" {
		return []string{"dark", "light"}
	}
	return []string{a.toolTheme()}
}

// overrideTheme applies TUIMARK_THEME for one Run session and returns the
// function that puts the previous value back. An empty name changes
// nothing.
func (a *App) overrideTheme(name string) (undo func()) {
	if name == "" {
		return func() {}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	prev := a.runEnvTheme
	a.runEnvTheme = name
	return func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		a.runEnvTheme = prev
	}
}

// resolveAuto records what theme="auto" (or @theme "auto") means for the
// rest of a Run session (SPEC §26.4): the probe's background when it
// recorded one, else the COLORFGBG theme, else dark. It returns the
// theme and the source the log names (§26.12).
func (a *App) resolveAuto(probed, fgbg string) (theme, from string) {
	theme, from = "dark", "default"
	switch {
	case probed != "":
		theme, from = probed, "osc11"
	case fgbg != "":
		theme, from = fgbg, "COLORFGBG"
	}
	a.mu.Lock()
	a.runAuto = theme
	a.mu.Unlock()
	return theme, from
}

// endRun forgets what Run resolved from the terminal: Dump and Validate
// never see it (MUST 13), and neither does a later Frame.
func (a *App) endRun() {
	a.mu.Lock()
	a.runAuto = ""
	a.mu.Unlock()
}

// themeSource names where a Run frame's effective theme comes from, for
// the caps record of TUIMARK_LOG (SPEC §26.12): TUIMARK_THEME, @theme,
// osc11, COLORFGBG, document, or default. autoFrom is what resolveAuto
// returned. The caller holds a.mu.
func (a *App) themeSource(autoFrom string) string {
	switch {
	case a.runEnvTheme != "":
		return envTheme
	case a.hostTheme == "auto" || (a.hostTheme == "" && a.doc.Theme == "auto"):
		return autoFrom
	case a.hostTheme != "":
		return ThemePath
	case a.doc.Theme != "":
		return "document"
	}
	return "default"
}

// wantsTerminalTheme reports whether Run resolves the theme from the
// terminal (SPEC §26.2): TUIMARK_THEME is unset and the theme the host
// set with @theme before Run, else the document's, is auto. Only then
// does the probe carry OSC 11.
func (a *App) wantsTerminalTheme(env string) bool {
	if env != "" {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.hostTheme != "" {
		return a.hostTheme == "auto"
	}
	return a.doc.Theme == "auto"
}

// ThemeFromCOLORFGBG is the theme the COLORFGBG fallback gives (SPEC
// §26.4): the last ;-separated field, when it is a decimal integer from 0
// to 15, is the terminal's background in the §26.3 palette; 0-6 and 8
// are dark, 7 and 9-15 light. Anything else gives "" (no theme).
func ThemeFromCOLORFGBG(v string) string {
	fields := strings.Split(v, ";")
	last := fields[len(fields)-1]
	if last == "" || len(last) > 2 {
		return ""
	}
	for i := 0; i < len(last); i++ {
		if last[i] < '0' || last[i] > '9' {
			return ""
		}
	}
	n, err := strconv.Atoi(last)
	if err != nil || n > 15 {
		return ""
	}
	if n <= 6 || n == 8 {
		return "dark"
	}
	return "light"
}

// ThemeFromOSC11 is the theme an OSC 11 reply's SPEC gives (SPEC §26.2):
// rgb:R/G/B or rgba:R/G/B/A, each component 1 to 4 hexadecimal digits in
// either case, the alpha ignored. The background is dark when max(r, g,
// b) + min(r, g, b) < 1, each component divided by 16^d − 1 (d its digit
// count), compared exactly (HSL lightness below 0.5); otherwise light.
// Any other SPEC gives "".
func ThemeFromOSC11(spec string) string {
	var body string
	var n int
	switch {
	case strings.HasPrefix(spec, "rgb:"):
		body, n = spec[len("rgb:"):], 3
	case strings.HasPrefix(spec, "rgba:"):
		body, n = spec[len("rgba:"):], 4
	default:
		return ""
	}
	parts := strings.Split(body, "/")
	if len(parts) != n {
		return ""
	}
	comps := make([]*big.Rat, 0, 3)
	for i, p := range parts {
		if len(p) < 1 || len(p) > 4 {
			return ""
		}
		v, err := strconv.ParseUint(p, 16, 16)
		if err != nil {
			return ""
		}
		if i < 3 {
			den := int64(1)<<(4*len(p)) - 1
			comps = append(comps, big.NewRat(int64(v), den))
		}
	}
	lo, hi := comps[0], comps[0]
	for _, c := range comps[1:] {
		if c.Cmp(lo) < 0 {
			lo = c
		}
		if c.Cmp(hi) > 0 {
			hi = c
		}
	}
	if new(big.Rat).Add(lo, hi).Cmp(big.NewRat(1, 1)) < 0 {
		return "dark"
	}
	return "light"
}

// jsonText renders a JSON value compactly for an error message.
func jsonText(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}
