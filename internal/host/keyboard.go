package host

import "github.com/abdul-hamid-achik/tuimark/internal/ir"

// The session's keyboard (SPEC v0.4d §26.14.1, §26.14.3): the keys a
// terminal can send, which decides what <hints> shows.
const (
	// KeyboardKitty is a terminal with the Kitty keyboard protocol (flag 1
	// pushed): every key token of §8.1 can be sent. It is the keyboard of
	// every tool that renders without a terminal (dump, validate, inspect,
	// Dump, Validate) and the default of play.
	KeyboardKitty = "kitty"
	// KeyboardModifyOtherKeys is a terminal that was sent CSI > 4 ; 2 m
	// because it did not answer the Kitty query. The runtime cannot confirm
	// what it does, so it counts as a legacy terminal.
	KeyboardModifyOtherKeys = "modify-other-keys"
	// KeyboardLegacy is a terminal that cannot tell a modified enter from
	// enter, and whose alt keys are ESC and the key.
	KeyboardLegacy = "legacy"
)

// SetKeyboard sets the keyboard the frames are built for and returns the
// previous one ("" is the default, KeyboardKitty). Run sets it for its
// session and restores it, and play sets it for a session of
// --keyboard legacy (SPEC v0.4d §26.14.3).
func (a *App) SetKeyboard(kb string) (prev string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	prev = a.keyboard
	a.keyboard = kb
	return prev
}

// Modern reports whether the document is in the current language (a
// <tui> without version): the language whose decoder, tokens, and keyboard
// modes SPEC v0.4d §26.14 describes. A pinned document keeps tuimark
// 0.4.4's keys (decision 202).
func (a *App) Modern() bool { return a.doc.Lang.V4() }

// canSend reports whether a terminal with the keyboard kb can send the key
// token tok (SPEC v0.4d §26.14.3, decision 220). Under Kitty every token
// can be sent; under legacy and modifyOtherKeys every token but
// shift+enter, ctrl+enter, alt+O, alt+P, and alt+X, whose bytes are
// another key's or open a sequence.
func canSend(tok, kb string) bool {
	if kb == "" || kb == KeyboardKitty {
		return true
	}
	return !unsendableLegacy(tok)
}

// unsendableLegacy reports whether tok is one of the five tokens a legacy
// terminal cannot send.
func unsendableLegacy(tok string) bool {
	switch tok {
	case "shift+enter", "ctrl+enter", "alt+O", "alt+P", "alt+X":
		return true
	}
	return false
}

// UnsendableLegacy is unsendableLegacy for play, which refuses such a
// step under --keyboard legacy.
func UnsendableLegacy(tok string) bool { return unsendableLegacy(tok) }

// producible reports whether tok is a key the session of the document's
// language can send. A pinned document has no keyboard modes and no new
// tokens, so every one of its tokens counts.
func (a *App) producible(tok string) bool {
	if !a.doc.Lang.V4() {
		return true
	}
	return canSend(tok, a.keyboard)
}

// keyTokenOK reports whether tok is a key token of the document's
// language.
func (a *App) keyTokenOK(tok string) bool { return ir.ValidKeyLang(tok, a.doc.Lang) }
