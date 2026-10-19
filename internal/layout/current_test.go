package layout

import (
	"fmt"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/css"
	"github.com/abdul-hamid-achik/tuimark/internal/ir"
)

// The layout defaults of the current language (SPEC v0.4b §11.2-§11.4,
// §11.5.2, §14): the engine reads them from Engine.Lang and the cascade
// from the built-in sheet of that language. The tests of layout_test.go
// lay out with Lang 0, a pinned document, and keep pinning 0.4.1's
// layout; the end-to-end vectors are tests 156-171 and 221 in the root
// package (conformance_v04b_defaults_test.go).

// runLang is run for a document of language l.
func runLang(t *testing.T, l ir.Lang, root *Box, cols, rows int, modals ...*Box) *Engine {
	t.Helper()
	c := css.NewCascadeLang(nil, css.Env{Cols: cols, Rows: rows}, l)
	styleTree(c, root, nil)
	for _, m := range modals {
		styleTree(c, m, nil)
	}
	if len(c.Diags) != 0 {
		t.Fatalf("cascade: %v", c.Diags)
	}
	e := &Engine{File: "t.tui", V3: l.V3(), Lang: l}
	e.Layout(root, modals, cols, rows)
	return e
}

func msgsOf(e *Engine) string {
	var out []string
	for _, d := range e.Diags {
		out = append(out, d.Code+" "+d.Msg)
	}
	return strings.Join(out, "\n")
}

// The §11.4 cases whose result the current language changes, with their
// pinned results (TestAllocateMainAxis) beside them: the sibling-fr rule
// holds for viewports only, and an fr child's min/max take part in the
// split (the freeze), so what a clamped child frees is shared again.
func TestCurrentAllocateMainAxis(t *testing.T) {
	cases := []struct {
		name         string
		kids         []string // "tag" or "tag;decls" per child
		pinned, curr []string // "w@x" per child
	}{
		{"an unsized box next to fr stays auto", []string{"box", "box;width: 1fr"}, []string{"50@0", "50@50"}, []string{"0@0", "100@0"}},
		{"an unsized col next to fr stays auto", []string{"col", "box;width: 1fr"}, []string{"50@0", "50@50"}, []string{"0@0", "100@0"}},
		{"an unsized list next to fr is 1fr", []string{"list", "box;width: 1fr"}, []string{"50@0", "50@50"}, []string{"50@0", "50@50"}},
		{"an overflow: scroll box next to fr is 1fr", []string{"box;overflow: scroll", "box;width: 1fr"}, []string{"50@0", "50@50"}, []string{"50@0", "50@50"}},
		{"a viewport made 1fr makes no sibling 1fr", []string{"list", "list"}, []string{"0@0", "0@0"}, []string{"0@0", "0@0"}},
		{"what a max frees is shared again", []string{"box;width: 1fr; max-width: 10", "box;width: 1fr"}, []string{"10@0", "50@10"}, []string{"10@0", "90@10"}},
		{"a min takes from the others", []string{"box;width: 1fr; min-width: 70", "box;width: 1fr"}, []string{"70@0", "50@70"}, []string{"70@0", "30@70"}},
		{"violations that cancel freeze every child", []string{"box;width: 1fr; min-width: 60", "box;width: 1fr; max-width: 40"}, []string{"60@0", "40@60"}, []string{"60@0", "40@60"}},
		{"a min over what is left overflows, the rest get 0", []string{"box;width: 1fr; min-width: 120", "box;width: 1fr"}, []string{"120@0", "50@120"}, []string{"120@0", "0@120"}},
		{"nothing binds: one split", []string{"box;width: 1fr", "box;width: 2fr", "box;width: 1fr"}, []string{"25@0", "50@25", "25@75"}, []string{"25@0", "50@25", "25@75"}},
	}
	for _, c := range cases {
		for _, l := range []ir.Lang{ir.Lang3, ir.LangCurrent} {
			t.Run(fmt.Sprintf("%s/lang%d", c.name, l), func(t *testing.T) {
				var kids []*Box
				for i, k := range c.kids {
					tag, decls, _ := strings.Cut(k, ";")
					kids = append(kids, n(fmt.Sprintf("%s#k%d", tag, i), strings.TrimSpace(decls)))
				}
				root := n("screen", "", n("row#r", "width: 100; height: 5", kids...))
				runLang(t, l, root, 200, 10)
				want := c.curr
				if l != ir.LangCurrent {
					want = c.pinned
				}
				for i, w := range want {
					if got := fmt.Sprintf("%d@%d", kids[i].W, kids[i].X); got != w {
						t.Errorf("child %d (%s) = %s, want %s", i, c.kids[i], got, w)
					}
				}
			})
		}
	}
}

// The current language's sheet: col and row are 1fr only as direct
// children of screen, modal, and tab, and auto (content-sized on the main
// axis, filling the cross axis) anywhere else (SPEC v0.4b §10.6, §11.2).
func TestCurrentColRowDefaults(t *testing.T) {
	root := n("screen#s", "",
		n("row#top", "",
			n("col#a", "", txt("ab", "")),
			n("box#b", "", n("col#inner", "", txt("abc", ""))),
		),
		n("col#bottom", "", txt("z", "")),
	)
	runLang(t, ir.LangCurrent, root, 20, 10)
	expectGeo(t, root, map[string]string{
		"top":    "20x5@(0,0)", // screen > row: 1fr on both axes
		"a":      "2x5@(0,0)",  // row > col: content width, fills down
		"b":      "3x5@(2,0)",
		"inner":  "3x1@(2,0)", // box > col: content height, fills across
		"bottom": "20x5@(0,5)",
	})
	// The direct children of modal and tab are 1fr too; a col elsewhere
	// has no size, and a pinned col is 1fr everywhere.
	for _, c := range []struct {
		parent string
		lang   ir.Lang
		want   ir.ScalarKind
	}{
		{"modal", ir.LangCurrent, ir.Fr},
		{"tab", ir.LangCurrent, ir.Fr},
		{"item", ir.LangCurrent, ir.Unset},
		{"box", ir.LangCurrent, ir.Unset},
		{"box", ir.Lang3, ir.Fr},
	} {
		k := n("col", "")
		styleTree(css.NewCascadeLang(nil, css.Env{Cols: 20, Rows: 10}, c.lang), n(c.parent, "", k), nil)
		if k.Style.Width.Kind != c.want || k.Style.Height.Kind != c.want {
			t.Errorf("%s > col in lang %d: %v x %v, want kind %v", c.parent, c.lang, k.Style.Width, k.Style.Height, c.want)
		}
	}
}

// L002 names the fix under a col or a row of the current language, and
// keeps the 0.4.1 text under any other parent and in a pinned document
// (decision 109).
func TestCurrentL002Messages(t *testing.T) {
	const fixed = "L002 height: 50% is a percent of a parent sized by its content (auto); give the parent a size (height: 1fr fills it)"
	const plain = "L002 height: 50% is a percent of a parent sized by its content (auto); give the parent a size"
	for _, c := range []struct {
		name   string
		lang   ir.Lang
		parent string
		want   string
	}{
		{"row, current", ir.LangCurrent, "row", fixed},
		{"col, current", ir.LangCurrent, "col", fixed},
		{"box, current", ir.LangCurrent, "box", plain},
		{"row with an auto height, pinned", ir.Lang3, "row", plain},
	} {
		t.Run(c.name, func(t *testing.T) {
			decl := ""
			if c.lang != ir.LangCurrent {
				decl = "height: auto"
			}
			p := n(c.parent+"#p", decl, n("box#half", "height: 50%; width: 1", txt("x", "")))
			root := n("screen", "", n("col#app", "height: 1fr", p))
			e := runLang(t, c.lang, root, 30, 8)
			got := ""
			for _, d := range e.Diags {
				if d.Code == "L002" {
					got = d.Code + " " + d.Msg
				}
			}
			if got != c.want {
				t.Errorf("got %q, want %q (%s)", got, c.want, msgsOf(e))
			}
		})
	}
	t.Run("modal sized by its content", func(t *testing.T) {
		m := n("modal#m", "", txt("abcd", ""), n("box#half", "width: 50%", txt("x", "")))
		root := n("screen", "")
		e := runLang(t, ir.LangCurrent, root, 30, 8, m)
		want := "L002 width: 50% is a percent of a parent sized by its content (auto); give the parent a size"
		if got := msgsOf(e); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}

// L009 says that a content-sized col, row, or box does not shrink, and
// which size makes it fit, only when §11.2 sized it by its content; a
// child with a size, docked, or in a pinned document keeps the 0.4.1
// advice (decision 109).
func TestCurrentL009Messages(t *testing.T) {
	cause := func(c, prop string) string {
		return fmt.Sprintf("; %s is sized by its content and does not shrink: give it %s: 1fr (or flex: 1), or write overflow: hidden to keep the cut", c, prop)
	}
	const room = "; give it room, or write overflow: hidden to keep the cut"
	for _, c := range []struct {
		name  string
		lang  ir.Lang
		child *Box
		want  string
	}{
		{"unsized col", ir.LangCurrent, n("col#c", "", txt("0123456789", "")), "L009 cuts #c on x: 4 of 10 cells shown" + cause("#c", "width")},
		{"unsized box", ir.LangCurrent, n("box", "", txt("0123456789", "")), "L009 cuts box on x: 4 of 10 cells shown" + cause("box", "width")},
		{"written auto width", ir.LangCurrent, n("col#c", "width: auto", txt("0123456789", "")), "L009 cuts #c on x: 4 of 10 cells shown" + room},
		{"a text", ir.LangCurrent, txt("0123456789", ""), "L009 cuts text[1] on x: 4 of 10 cells shown" + room},
		{"pinned", ir.Lang3, n("col#c", "width: auto", txt("0123456789", "")), "L009 cuts #c on x: 4 of 10 cells shown" + room},
	} {
		t.Run(c.name, func(t *testing.T) {
			root := n("screen", "", n("row#r", "", txt("x", "width: 6"), c.child))
			e := runLang(t, c.lang, root, 10, 3)
			if got := msgsOf(e); got != c.want && !strings.HasPrefix(got, c.want+"\n") {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
	t.Run("a cut on y", func(t *testing.T) {
		root := n("screen", "", n("col#app", "height: 2", txt("a", ""), n("row#r", "", txt("b\nc", ""))))
		e := runLang(t, ir.LangCurrent, root, 10, 3)
		want := "L009 cuts #r on y: 1 of 2 cells shown" + cause("#r", "height")
		if got := msgsOf(e); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}

// In the current language a modal with no size takes its content size,
// capped at 80% of the root or at a written max, its title counting as
// width(title) + 6; an explicit auto is capped by the root alone (SPEC
// v0.4b §11.3, decisions 30 and 31).
func TestCurrentModalSize(t *testing.T) {
	body := func() []*Box {
		var out []*Box
		for i := 0; i < 12; i++ {
			out = append(out, txt(fmt.Sprintf("line %d", i+1), ""))
		}
		return out
	}
	for _, c := range []struct {
		name, decls, title string
		lang               ir.Lang
		want               string
	}{
		{"content size", "", "", ir.LangCurrent, "9x9@(15,1)"},
		{"title counts", "", "a long help title here", ir.LangCurrent, "28x9@(6,1)"},
		{"explicit auto", "width: auto; height: auto", "a long help title here", ir.LangCurrent, "28x12@(6,0)"},
		{"written max", "max-height: 100%", "", ir.LangCurrent, "9x12@(15,0)"},
		{"written max in cells", "max-width: 5; max-height: 4", "", ir.LangCurrent, "5x4@(17,4)"},
		{"min raises", "min-width: 30", "", ir.LangCurrent, "30x9@(5,1)"},
		{"pinned 80%", "", "a long help title here", ir.Lang3, "32x9@(4,1)"},
		{"pinned explicit auto ignores the title", "width: auto; height: auto", "a long help title here", ir.Lang3, "9x12@(15,0)"},
	} {
		t.Run(c.name, func(t *testing.T) {
			m := n("modal#m", c.decls, body()...)
			m.Title = c.title
			runLang(t, c.lang, n("screen", ""), 40, 12, m)
			if got := geo(m); got != c.want {
				t.Errorf("#m = %s, want %s", got, c.want)
			}
		})
	}
	t.Run("no border, no title minimum", func(t *testing.T) {
		m := n("modal#m", "border: none", txt("ab", ""))
		m.Title = "a long title"
		runLang(t, ir.LangCurrent, n("screen", ""), 40, 12, m)
		if got := geo(m); got != "2x1@(19,5)" {
			t.Errorf("#m = %s, want 2x1@(19,5)", got)
		}
	})
}

// An input's intrinsic width ignores its value in the current language:
// max(width(placeholder) + 1, 20), a secret's included (SPEC v0.4b
// §11.5.2, decision 99).
func TestCurrentInputWidth(t *testing.T) {
	for _, c := range []struct {
		value, placeholder string
		secret             bool
		lang               ir.Lang
		want               int
	}{
		{"", "", false, ir.LangCurrent, 20},
		{"a value longer than twenty columns", "", false, ir.LangCurrent, 20},
		{"", "type a new name here", false, ir.LangCurrent, 21},
		{"abc", "", true, ir.LangCurrent, 20},
		{"abc", "", false, ir.Lang3, 4},
		{"", "type a new name here", false, ir.Lang3, 21},
	} {
		in := n("input#i", "")
		in.Text, in.Placeholder, in.Secret = c.value, c.placeholder, c.secret
		runLang(t, c.lang, n("screen", "", n("row#r", "height: 1", in)), 60, 3)
		if in.W != c.want {
			t.Errorf("%+v: width %d, want %d", c, in.W, c.want)
		}
	}
}

// A hints is shrinkable along its items' axis in the current language,
// so it drops the items that do not fit instead of being cut, and a
// spacer gets what it leaves; in a pinned document it is a leaf (SPEC
// v0.4b §6.12, §11.4, decision 29).
func TestCurrentHintsShrink(t *testing.T) {
	items := func() []*Box {
		var out []*Box
		for _, kl := range [][2]string{{"q", "quit"}, {"/", "find"}, {"r", "reload"}} {
			k, l := txt(kl[0], ""), txt(kl[1], "")
			k.Fixed, l.Fixed = true, true
			it := n("row", "", k, l)
			it.Fixed, it.Role = true, RoleHintItem
			out = append(out, it)
		}
		return out
	}
	h := n("hints#keys", "", items()...)
	root := n("screen", "", n("row#r", "height: 1", txt("ready", ""), n("spacer#sp", ""), h))
	e := runLang(t, ir.LangCurrent, root, 20, 1)
	expectGeo(t, root, map[string]string{"keys": "15x1@(5,0)", "sp": "0x1@(5,0)"})
	laid := 0
	for _, it := range h.Children {
		if it.Laid {
			laid++
		}
	}
	if laid != 2 || len(e.Diags) != 0 {
		t.Errorf("%d items laid out, want 2; diagnostics %v", laid, e.Diags)
	}
	if !e.shrinkable(h, true) || e.shrinkable(h, false) {
		t.Error("a row hints is shrinkable on x only")
	}
	hc := n("hints#c", "layout: column", items()...)
	runLang(t, ir.LangCurrent, n("screen", "", hc), 20, 1)
	if e2 := (&Engine{Lang: ir.LangCurrent}); !e2.shrinkable(hc, false) || e2.shrinkable(hc, true) {
		t.Error("a column hints is shrinkable on y only")
	}
	if e3 := (&Engine{Lang: ir.Lang3}); e3.shrinkable(h, true) || e3.shrinkable(h, false) {
		t.Error("a pinned hints is never shrinkable")
	}
}
