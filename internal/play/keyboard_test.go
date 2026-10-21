package play

import (
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/host"
	"github.com/abdul-hamid-achik/tuimark/internal/ir"
)

// SPEC v0.4d test 275 (what 0.4d leaves alone): for every key token of
// tuimark 0.4.5, a key step in a document of the current language, sent as
// the bytes of either keyboard and decoded by the decoder of the current
// language, is the key tuimark 0.4.5 delivered for it (KeyFromToken, the
// path a pinned document keeps), so no play session of a valid 0.4.5
// document changes. Only the LF mark of ctrl+j, which 0.4.5 had no use
// for, differs.
func TestKeyStepsKeepTheirKeys(t *testing.T) {
	var toks []string
	toks = append(toks, ir.NamedKeys...)
	for c := 'a'; c <= 'z'; c++ {
		toks = append(toks, "ctrl+"+string(c))
	}
	for c := '!'; c < 0x7f; c++ {
		if c != ',' {
			toks = append(toks, string(c))
		}
	}
	for _, kb := range []string{host.KeyboardKitty, host.KeyboardLegacy} {
		for _, tok := range toks {
			b, ok := host.EncodeKey(tok, kb)
			if !ok {
				t.Errorf("%s: no bytes for %q", kb, tok)
				continue
			}
			ins := host.DecodeInputLang([]byte(b), true)
			if len(ins) != 1 {
				t.Errorf("%s: %q -> %q -> %+v", kb, tok, b, ins)
				continue
			}
			got, want := ins[0].Key, KeyFromToken(tok)
			got.LF = false
			if got != want {
				t.Errorf("%s: %q -> %q -> %+v, 0.4.5 delivered %+v", kb, tok, b, got, want)
			}
		}
	}
}
