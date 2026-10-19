package css

import (
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/ir"
)

// SPEC v0.4b §10.6: the current language's built-in sheet is UASheet with
// its col and row rules replaced by three rules, and the form-control rule
// and the focus rule added at its end (v0.4b §12.5: checkbox); every other
// rule is UASheet's, in order. Pinned documents
// keep UASheet as it was in 0.4.1.
func TestUASheetCurrentIsUASheetWithTheCurrentRules(t *testing.T) {
	const old = "col    { layout: column; width: 1fr; height: 1fr; }\nrow    { layout: row;    width: 1fr; height: 1fr; }\n"
	const repl = "col    { layout: column; }\nrow    { layout: row; }\n" +
		"screen > col, screen > row, modal > col, modal > row, tab > col, tab > row { width: 1fr; height: 1fr; }\n"
	if strings.Count(UASheet, old) != 1 {
		t.Fatal("UASheet's col and row rules moved")
	}
	want := strings.Replace(UASheet, old, repl, 1) +
		"checkbox { width: auto; height: auto; wrap: truncate; }\n" +
		"button:focus, checkbox:focus { reverse: true; }\n"
	if UASheetCurrent != want {
		t.Errorf("UASheetCurrent drifted from UASheet:\n%s", UASheetCurrent)
	}
	if UALang(ir.LangCurrent) == UA() || UALang(ir.Lang3) != UA() || UALang(0) != UA() {
		t.Error("UALang picks the wrong sheet")
	}
}

// The current language's col/row defaults and focus look, through the
// cascade (SPEC v0.4b §10.6): 1fr only under screen, modal, and tab; a
// focused button reversed, which an author rule overrides by origin.
func TestUASheetCurrentCascade(t *testing.T) {
	compute := func(l ir.Lang, author string, chain ...*el) Style {
		var sheets []*Sheet
		if author != "" {
			sh, diags := ParseSheetLang(author, "a.tcss", l)
			if len(diags) != 0 {
				t.Fatal(diags)
			}
			sheets = append(sheets, sh)
		}
		c := NewCascadeLang(sheets, Env{Cols: 80, Rows: 24}, l)
		var parent *Style
		var st Style
		for i, e := range chain {
			if i > 0 {
				e.parent = chain[i-1]
			}
			st = c.Compute(e, parent, nil, nil)
			p := st
			parent = &p
		}
		return st
	}
	for _, c := range []struct {
		parent string
		lang   ir.Lang
		fr     bool
	}{
		{"screen", ir.LangCurrent, true},
		{"modal", ir.LangCurrent, true},
		{"tab", ir.LangCurrent, true},
		{"col", ir.LangCurrent, false},
		{"box", ir.LangCurrent, false},
		{"box", ir.Lang3, true},
		{"box", ir.Lang1, true},
	} {
		for _, tag := range []string{"col", "row"} {
			st := compute(c.lang, "", &el{tag: c.parent}, &el{tag: tag})
			if fr := st.Width.Kind == ir.Fr && st.Height.Kind == ir.Fr && st.WidthUA && st.HeightUA; fr != c.fr {
				t.Errorf("%s > %s, lang %d: width %v height %v, want 1fr: %v", c.parent, tag, c.lang, st.Width, st.Height, c.fr)
			}
			if st.Width.Kind != ir.Fr && (st.Width.Kind != ir.Unset || st.Height.Kind != ir.Unset) {
				t.Errorf("%s > %s, lang %d: an unsized col/row has no size, got %v x %v", c.parent, tag, c.lang, st.Width, st.Height)
			}
		}
	}
	focused := &el{tag: "button", pseudos: []string{"focus"}}
	if st := compute(ir.LangCurrent, "", &el{tag: "screen"}, focused); !st.Reverse {
		t.Error("a focused button is not reversed in the current language")
	}
	if st := compute(ir.Lang3, "", &el{tag: "screen"}, focused); st.Reverse {
		t.Error("a focused button is reversed in a pinned document")
	}
	if st := compute(ir.LangCurrent, "", &el{tag: "screen"}, &el{tag: "button"}); st.Reverse {
		t.Error("an unfocused button is reversed")
	}
	if st := compute(ir.LangCurrent, "button { reverse: false; }", &el{tag: "screen"}, focused); st.Reverse {
		t.Error("an author rule does not win over the focus look")
	}
	if st := compute(ir.LangCurrent, "", &el{tag: "screen"}, &el{tag: "input", pseudos: []string{"focus"}}); st.Reverse {
		t.Error("a focused input is reversed")
	}
}
