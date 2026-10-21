package paint

import (
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/css"
	"github.com/abdul-hamid-achik/tuimark/internal/ir"
	"github.com/abdul-hamid-achik/tuimark/internal/layout"
)

// runsText is a text whose content is the joined text of the runs, each run
// bold when its bold flag is set: the shape the host gives a <text runs>.
func runsText(decls string, runs ...[2]string) *layout.Box {
	b := tx("", "")
	if decls != "" {
		// The current language, which accepts wrap: pre-wrap.
		ds, errs := css.ParseDeclsAllLang(decls, ir.Lang4)
		if len(errs) != 0 {
			panic(errs[0])
		}
		b.Inline = ds
	}
	rs := &layout.TextRuns{}
	for i, r := range runs {
		b.Text += r[0]
		sp := layout.RunSpan{End: len(b.Text), Index: i}
		if r[1] == "bold" {
			sp.Style = &css.Style{Bold: true}
		}
		rs.Spans = append(rs.Spans, sp)
	}
	b.Runs = rs
	return b
}

func attrsAt(g *Grid, x, y int) uint8 { return g.At(x, y).Attrs }

// SPEC 0.4d §6.20.2, §6.20.5: a cluster takes the look of the run of its
// first code point, so a combining mark in the next run is painted with its
// base, and a wide cluster is painted whole in its run's look.
func TestRunsClusterTakesTheLookOfItsFirstCodePoint(t *testing.T) {
	root := bx("screen", "", runsText("", [2]string{"e", "bold"}, [2]string{"́x", ""}, [2]string{"微", "bold"}, [2]string{"y", ""}))
	g := render(t, root, 8, 1)
	if got := g.Lines()[0]; got != "éx微y   " {
		t.Fatalf("row %q", got)
	}
	// Columns: e+acute (bold), x, 微 (2 columns, bold), y.
	want := []uint8{Bold, 0, Bold, Bold, 0}
	for x, w := range want {
		if got := attrsAt(g, x, 0); got != w {
			t.Errorf("attrs at %d = %b, want %b", x, got, w)
		}
	}
}

// §6.20.5: the joining space of wrap: wrap has the look of the first space
// it stands for; the ellipsis has the text's own look.
func TestRunsJoiningSpaceAndEllipsis(t *testing.T) {
	// "ab" bold, " " plain, "cd" bold: wrapped at 5 the line is "ab cd" and
	// the joining space stands for the plain space.
	root := bx("screen", "", runsText("wrap: wrap", [2]string{"ab", "bold"}, [2]string{" ", ""}, [2]string{"cd", "bold"}))
	g := render(t, root, 6, 1)
	if got := g.Lines()[0]; got != "ab cd " {
		t.Fatalf("row %q", got)
	}
	for x, w := range []uint8{Bold, Bold, 0, Bold, Bold} {
		if got := attrsAt(g, x, 0); got != w {
			t.Errorf("wrap: attrs at %d = %b, want %b", x, got, w)
		}
	}
	// Truncated at 4 columns: "ab…" is "ab" bold and the ellipsis plain.
	root = bx("screen", "", runsText("wrap: truncate; width: 4", [2]string{"abc", "bold"}, [2]string{"def", ""}))
	g = render(t, root, 6, 1)
	if got := g.Lines()[0]; got != "abc…  " {
		t.Fatalf("truncate row %q", got)
	}
	for x, w := range []uint8{Bold, Bold, Bold, 0} {
		if got := attrsAt(g, x, 0); got != w {
			t.Errorf("truncate: attrs at %d = %b, want %b", x, got, w)
		}
	}
}

// pre-wrap and content-align: a line is aligned without its hanging spaces,
// and the run keeps its look across the spaces.
func TestRunsPreWrapAlignsWithoutTrailingSpaces(t *testing.T) {
	root := bx("screen", "", runsText("wrap: pre-wrap; content-align: end; width: 6", [2]string{"ab", "bold"}, [2]string{"    ", ""}, [2]string{"cd", "bold"}))
	g := render(t, root, 8, 3)
	got := g.Lines()
	if got[0] != "    ab  " || got[1] != "    cd  " {
		t.Fatalf("rows %q", got)
	}
}

// A text with runs whose runs are all empty paints nothing, and a text with
// no style on its spans paints as its own text would.
func TestRunsWithoutLookArePlain(t *testing.T) {
	root := bx("screen", "", runsText("", [2]string{"plain", ""}))
	g := render(t, root, 8, 1)
	if got := g.Lines()[0]; got != "plain   " {
		t.Errorf("row %q", got)
	}
	for x := 0; x < 5; x++ {
		if attrsAt(g, x, 0) != 0 {
			t.Errorf("attrs at %d", x)
		}
	}
}
