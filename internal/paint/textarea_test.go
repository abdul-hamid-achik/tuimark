package paint

import (
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/layout"
)

func taBox(id, text string, cursor int, decls string) *layout.Box {
	b := bx("textarea#"+id, decls)
	b.Text, b.Cursor = text, cursor
	return b
}

// SPEC 0.4d §12.8, §6.19.5: the visual lines from the offset fill the
// content box, one per row; the spaces that hang past the width are not
// painted; the placeholder is painted dim; the terminal's cursor goes on
// the caret when the textarea has focus, and is off when the caret's line
// is outside the box.
func TestTextareaPaint(t *testing.T) {
	// Hanging spaces are cut at the content box.
	tb := taBox("t", "ab         cd", 13, "")
	tb.Focused = true
	tb.Follow = 0
	g := render(t, bx("screen", "", tb), 10, 3)
	if got := g.Lines(); got[0] != "ab        " || got[1] != "cd        " || got[2] != "          " {
		t.Errorf("rows %q", got)
	}
	if !g.CursorOn || g.CursorX != 2 || g.CursorY != 1 {
		t.Errorf("cursor %v at (%d, %d), want (2, 1)", g.CursorOn, g.CursorX, g.CursorY)
	}
	// An unfocused textarea has no cursor.
	tb = taBox("t", "abc", 3, "")
	if g := render(t, bx("screen", "", tb), 10, 3); g.CursorOn {
		t.Error("an unfocused textarea shows the cursor")
	}
	// A disabled one neither.
	tb = taBox("t", "abc", 3, "")
	tb.Focused, tb.Disabled = true, true
	if g := render(t, bx("screen", "", tb), 10, 3); g.CursorOn {
		t.Error("a disabled textarea shows the cursor")
	}
	// The placeholder is dim, cut at the content width, and the cursor
	// stands at the origin.
	tb = taBox("t", "", 0, "")
	tb.Placeholder, tb.Focused = "Message of a long kind", true
	g = render(t, bx("screen", "", tb), 10, 2)
	if got := g.Lines()[0]; got != "Message of" {
		t.Errorf("placeholder %q", got)
	}
	if attrsAt(g, 0, 0)&Dim == 0 || attrsAt(g, 9, 0)&Dim == 0 {
		t.Error("the placeholder is not dim")
	}
	if !g.CursorOn || g.CursorX != 0 || g.CursorY != 0 {
		t.Errorf("cursor at (%d, %d)", g.CursorX, g.CursorY)
	}
	// The text of a value is not dim.
	tb = taBox("t", "hi", 2, "")
	if g := render(t, bx("screen", "", tb), 10, 2); attrsAt(g, 0, 0)&Dim != 0 {
		t.Error("text is dim")
	}

	// From the offset on; the caret above the box hides the cursor.
	tb = taBox("t", "1\n2\n3\n4\n5", 0, "height: 2")
	tb.Focused, tb.ScrollY = true, 3
	g = render(t, bx("screen", "", tb), 10, 3)
	if got := g.Lines(); got[0] != "4         " || got[1] != "5         " {
		t.Errorf("rows from the offset %q", got)
	}
	if g.CursorOn {
		t.Error("the cursor is shown with the caret outside the content box")
	}
	// Wide clusters are painted whole and a border and padding frame the box.
	tb = taBox("t", "微信 微信", 5, "border: single; padding: 0 1")
	tb.Focused, tb.Follow = true, 0
	g = render(t, bx("screen", "", tb), 9, 4)
	if got := strings.Join(g.Lines(), "\n"); !strings.Contains(got, "┌───────┐") || !strings.Contains(got, "│ 微信  │") {
		t.Errorf("framed:\n%s", got)
	}
}
