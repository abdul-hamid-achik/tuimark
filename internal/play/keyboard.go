package play

import (
	"fmt"

	"github.com/abdul-hamid-achik/tuimark/internal/host"
	"github.com/abdul-hamid-achik/tuimark/internal/ir"
)

// The keyboard `play` emulates (SPEC v0.4d §15.4, §26.14.3, decision 221).
// For a document in the current language a key step is delivered as the
// bytes that terminal sends for the token, decoded by §26.14.2: Kitty (the
// default, also for "") sends the CSI-u form for the keys that have one,
// and legacy the legacy bytes. A document that writes version ignores the
// keyboard and gets tuimark 0.4.4's key.

// ValidKeyboard reports whether kb is a keyboard play knows: "kitty", the
// default (also ""), or "legacy".
func ValidKeyboard(kb string) bool {
	return kb == "" || kb == host.KeyboardKitty || kb == host.KeyboardLegacy
}

// CheckKeys checks the key steps against the document and the keyboard
// before any step is applied (SPEC v0.4d §15.4): in a document that writes
// version, a token of the current language is the §5.1 message; under
// legacy, a key a legacy terminal cannot send is a usage error. The error
// is "step N (RAW): REASON", the form play and Play report. keyboard is
// ignored for a pinned document.
func CheckKeys(app *host.App, steps []Step, keyboard string) error {
	modern := app.Modern()
	for i, st := range steps {
		if st.Kind != KindKey {
			continue
		}
		switch {
		case !modern && !ir.ValidKey(st.Tok):
			return fmt.Errorf("step %d (%s): key %q%s", i+1, st.Raw, st.Tok, ir.VersionHintCurrent)
		case modern && keyboard == host.KeyboardLegacy && host.UnsendableLegacy(st.Tok):
			return fmt.Errorf("step %d (%s): key %q cannot be sent by a legacy terminal (--keyboard legacy)", i+1, st.Raw, st.Tok)
		}
	}
	return nil
}
