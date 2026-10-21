package tuimark_test

import (
	"testing"

	"github.com/abdul-hamid-achik/tuimark"
)

// SPEC v0.4d §21 tests of phase D5a-2 through the Go API.

// SPEC 0.4d test 302, its last bullet: a focused textarea bound to s moves
// its caret to the end after an append (§6.19.2, §18.3), whether it is a
// plain textarea or the one inside a template's copy. The caret is moved
// to the start first, so the end is the append's doing.
func TestD5a2AppendMovesTextareaCaret302(t *testing.T) {
	for _, c := range []struct{ id, src string }{
		{"t", `<tui><screen id="main" focus="#t"><textarea id="t" bind="s"/></screen></tui>`},
		{"c__t", `<tui>
  <template id="ta"><textarea id="t" bind="v"/></template>
  <screen id="main" focus="#c__t"><use id="c" template="ta" with="s as v"/></screen>
</tui>`},
	} {
		caret := func(steps ...string) (line, col int) {
			t.Helper()
			app := parseDoc(t, c.src)
			bindJSON(t, app, "", `{"s": "ab", "a": [1], "n": 3, "o": {}, "z": null}`)
			res, err := app.Play(tuimark.PlayOptions{Cols: 30, Rows: 6}, steps...)
			if err != nil {
				t.Fatal(err)
			}
			n := dumpNode(res.Dump, c.id)
			if n == nil || n.Caret == nil {
				t.Fatalf("%s: no caret on %s", c.src, c.id)
			}
			return n.Caret.Line, n.Caret.Col
		}
		if l, col := caret("home"); l != 0 || col != 0 {
			t.Fatalf("%s: home puts the caret at (%d, %d), want (0, 0)", c.id, l, col)
		}
		if l, col := caret("home", `append:s="c"`); l != 0 || col != 3 {
			t.Errorf("%s: after home and an append the caret is at (%d, %d), want (0, 3)", c.id, l, col)
		}
		// A line break in the appended text puts the caret on the new line.
		if l, col := caret("home", `append:s="\nxy"`); l != 1 || col != 2 {
			t.Errorf("%s: after an append with a line break the caret is at (%d, %d), want (1, 2)", c.id, l, col)
		}
		// A write of the value it already holds is no change: the caret stays.
		if l, col := caret("home", `set:s="ab"`); l != 0 || col != 0 {
			t.Errorf("%s: a set of the same value moved the caret to (%d, %d)", c.id, l, col)
		}
	}
}
