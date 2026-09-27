package host

import (
	"fmt"

	"github.com/abdul-hamid-achik/tuimark/internal/css"
	"github.com/abdul-hamid-achik/tuimark/internal/ir"
)

// SetTheme makes the app render with the built-in theme name ("dark" or
// "light") instead of the document's theme attribute (SPEC v0.2 §26.4):
// the token set that :root tokens then override, for Frame, Dump,
// Validate, and Static alike, including the static token check (V003 for
// a token the theme does not define). It is the hook behind the CLI's
// --theme and Run's TUIMARK_THEME; it is not public API. "auto" and any
// other value are errors (theme="auto" is 0.2b).
func (a *App) SetTheme(name string) error {
	if name != "dark" && name != "light" {
		return fmt.Errorf("tuimark: unknown theme %q (want dark or light)", name)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.setTheme(name)
	return nil
}

// Theme returns the effective theme: the one SetTheme chose, else the
// document's theme attribute, else "dark".
func (a *App) Theme() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.doc.Theme == "" {
		return "dark"
	}
	return a.doc.Theme
}

// setTheme swaps the theme the cascade and the static token check use and
// re-runs that check. The caller holds a.mu.
func (a *App) setTheme(name string) {
	old := a.doc.Theme
	if old == name {
		return
	}
	// Replace the token check's diagnostics (made at load time for the old
	// theme) with the new theme's; other static diagnostics stay.
	drop := map[string]int{}
	for _, d := range css.CheckTokens(a.sheets, old, a.doc.InlineDecls()...) {
		drop[d.String()]++
	}
	kept := make(ir.Diags, 0, len(a.static))
	for _, d := range a.static {
		if k := d.String(); drop[k] > 0 {
			drop[k]--
			continue
		}
		kept = append(kept, d)
	}
	a.doc.Theme = name
	a.static = append(kept, css.CheckTokens(a.sheets, name, a.doc.InlineDecls()...)...)
}

// overrideTheme applies TUIMARK_THEME for one Run session and returns the
// function that puts the previous theme back. An empty name changes
// nothing.
func (a *App) overrideTheme(name string) (undo func()) {
	if name == "" {
		return func() {}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	prev := a.doc.Theme
	a.setTheme(name)
	return func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		a.setTheme(prev)
	}
}
