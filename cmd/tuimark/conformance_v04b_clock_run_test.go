package main

import (
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/host"
	"github.com/abdul-hamid-achik/tuimark/internal/paint"
)

// SPEC v0.4b §24 test 196 (phase F11), the CLI side: the readers never
// turn a spinner. The Run side (tests 194 and 195, the Dump called while
// Run is active) is internal/host's runclock_*_test.go.

// 196. preview paints phase 0 and braille, whatever --color says; inspect
// names the spinner's node; validate reports nothing.
func TestV04bClock196ReadersNeverTurn(t *testing.T) {
	code, out, errw := runCLI("preview", clockS, "--cols", "20", "--rows", "5")
	if code != 0 {
		t.Fatalf("preview: exit %d: %s", code, errw)
	}
	if !strings.Contains(out, "⠋ loading") {
		t.Errorf("preview did not paint phase 0:\n%s", out)
	}

	// preview --color 16 on a TTY: the text dump has the braille glyph,
	// and so does the ANSI grid it paints: the ASCII set is Run()'s alone.
	app, err := host.Load(clockS)
	if err != nil {
		t.Fatal(err)
	}
	var tty strings.Builder
	if !renderPreview(&tty, app, 20, 5, true, paint.ANSI16) {
		t.Fatal("renderPreview reported !ok for S.tui")
	}
	ansi, text, found := strings.Cut(tty.String(), "=== grid")
	if !found {
		t.Fatalf("no text dump in the TTY preview:\n%s", tty.String())
	}
	if !strings.Contains(ansi, "\x1b[") || !strings.Contains(ansi, "⠋") || strings.ContainsAny(ansi, "|/\\") {
		t.Errorf("the ANSI grid of preview --color 16 is not braille:\n%q", ansi)
	}
	if !strings.Contains(text, "⠋ loading") {
		t.Errorf("the text dump of preview --color 16 is not braille:\n%s", text)
	}

	code, out, errw = runCLI("inspect", clockS, "--at", "0,0", "--json")
	if code != 0 {
		t.Fatalf("inspect: exit %d: %s", code, errw)
	}
	if !strings.Contains(out, `"sp"`) {
		t.Errorf("inspect --at 0,0 does not name node sp:\n%s", out)
	}

	code, out, errw = runCLI("validate", clockS)
	if code != 0 || strings.TrimSpace(out) != "ok (bindings not checked: no --data)" || errw != "" {
		t.Errorf("validate: exit %d, stdout %q, stderr %q", code, out, errw)
	}
}
