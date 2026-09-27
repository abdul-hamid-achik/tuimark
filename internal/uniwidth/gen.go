//go:build ignore

// gen.go writes tables.go: the per-code-point data behind RuneWidth,
// ClusterWidth, and Next (SPEC v0.2 §11.5.1), derived from
// github.com/rivo/uniseg, whose tables are Unicode 15.0.0: its public
// width functions and, for the properties they do not expose
// (Grapheme_Cluster_Break and East_Asian_Width), the generated property
// tables in its source. It sets uniseg.EastAsianAmbiguousWidth, which is
// allowed here because this program is a build tool; the runtime never
// reads or writes that variable.
//
//	go run gen.go
//
// Regenerating after a uniseg upgrade is a spec change (§11.5.1): the
// oracle tests in uniwidth_test.go must still pass and the frozen goldens
// must not move.
package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"

	"github.com/rivo/uniseg"
)

type span struct{ lo, hi rune }

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "gen: "+format+"\n", args...)
	os.Exit(1)
}

// sourceTable reads the [][3]int property table named name from file in
// uniseg's module directory: for each code point of the table, its
// property constant's name (prControl, prW, ...). Code points the table
// does not list are "".
func sourceTable(dir, file, name string) []string {
	f, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, file), nil, 0)
	if err != nil {
		fail("%v", err)
	}
	props := make([]string, 0x110000)
	found := false
	ast.Inspect(f, func(n ast.Node) bool {
		vs, ok := n.(*ast.ValueSpec)
		if !ok || len(vs.Names) != 1 || vs.Names[0].Name != name || len(vs.Values) != 1 {
			return true
		}
		found = true
		for _, e := range vs.Values[0].(*ast.CompositeLit).Elts {
			row := e.(*ast.CompositeLit).Elts
			lo, err1 := strconv.ParseInt(row[0].(*ast.BasicLit).Value, 0, 32)
			hi, err2 := strconv.ParseInt(row[1].(*ast.BasicLit).Value, 0, 32)
			if err1 != nil || err2 != nil {
				fail("%s: bad range %v", name, row)
			}
			for r := lo; r <= hi; r++ {
				props[r] = row[2].(*ast.Ident).Name
			}
		}
		return false
	})
	if !found {
		fail("%s not found in %s", name, file)
	}
	return props
}

// eawWidePictographs are the Extended_Pictographic code points without
// Emoji_Presentation whose East_Asian_Width is W or F in Unicode 15.0.0.
// rw rule 3 of §11.5.1 gives them 2, where uniseg gives 1; the spec lists
// them, so a uniseg upgrade that changes the list must change the spec.
var eawWidePictographs = []rune{0x3030, 0x303D, 0x3297, 0x3299, 0x1F202, 0x1F237, 0x1F260, 0x1F261, 0x1F262, 0x1F263, 0x1F264, 0x1F265}

// standIns are the code points uniwidth.Next segments in place of East
// Asian Ambiguous ones, by UTF-8 length (a length-1 unit is an invalid
// byte, which decodes to U+FFFD): Grapheme_Cluster_Break Other and a
// width uniseg computes without reading EastAsianAmbiguousWidth. They
// must match standIn in uniwidth.go.
var standIns = map[int]rune{1: 'a', 2: 0x00A2, 3: 0x4E00, 4: 0x20000}

func main() {
	out, err := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", "github.com/rivo/uniseg").Output()
	if err != nil {
		fail("go list: %v", err)
	}
	dir := strings.TrimSpace(string(out))
	gcb := sourceTable(dir, "graphemeproperties.go", "graphemeCodePoints")
	eaw := sourceTable(dir, "eastasianwidth.go", "eastAsianWidth")

	width := func(s string, global int) int {
		uniseg.EastAsianAmbiguousWidth = global
		return uniseg.StringWidth(s)
	}
	var zero, wide, pict, amb, joiner, follower, eawPict []span
	add := func(list []span, r rune) []span {
		if n := len(list); n > 0 && list[n-1].hi == r-1 {
			list[n-1].hi = r
			return list
		}
		return append(list, span{r, r})
	}
	for r := rune(0); r <= 0x10FFFF; r++ {
		if r >= 0xD800 && r <= 0xDFFF {
			continue // surrogates are not valid in UTF-8
		}
		s := string(r)
		// Only a cluster led by an Extended_Pictographic code point takes
		// its width from a variation selector: VS16 makes it 2, VS15 1.
		// Every other lead gives equal widths for both (the selectors are
		// width-0 Extend code points).
		isPict := width(s+"\uFE0F", 1) == 2 && width(s+"\uFE0E", 1) == 1
		if isPict != (gcb[r] == "prExtendedPictographic") {
			fail("%U: variation selectors and Grapheme_Cluster_Break disagree on Extended_Pictographic", r)
		}
		if isPict {
			pict = add(pict, r)
		}
		// A single code point is its own cluster: uniseg reports rw(r),
		// with 3 and 4 for U+2E3A and U+2E3B, which §11.5.1 caps at 2, and
		// 1 for the Extended_Pictographic code points without
		// Emoji_Presentation, which rule 3 makes 2 when East_Asian_Width
		// is W or F.
		w := width(s, 1)
		if isPict && w == 1 && (eaw[r] == "prW" || eaw[r] == "prF") {
			eawPict = add(eawPict, r)
			w = 2
		}
		switch {
		case w == 0:
			zero = add(zero, r)
		case w >= 2:
			wide = add(wide, r)
		}
		// uniseg's width lookup for r reads EastAsianAmbiguousWidth exactly
		// when r's lone width changes with it.
		if width(s, 1) != width(s, 2) {
			if gcb[r] != "" && gcb[r] != "prAny" {
				fail("%U: East Asian Ambiguous with Grapheme_Cluster_Break %s; the stand-ins assume Other", r, gcb[r])
			}
			amb = add(amb, r)
		}
		switch gcb[r] {
		case "prExtend", "prZWJ", "prSpacingMark":
			follower = add(follower, r)
			joiner = add(joiner, r)
		case "prPrepend", "prRegionalIndicator", "prL", "prV", "prT", "prLV", "prLVT", "prExtendedPictographic":
			joiner = add(joiner, r)
		case "", "prAny", "prControl", "prCR", "prLF":
		default:
			fail("%U: unknown Grapheme_Cluster_Break %s", r, gcb[r])
		}
	}
	var listed []span
	for _, r := range eawWidePictographs {
		listed = add(listed, r)
	}
	if fmt.Sprint(listed) != fmt.Sprint(eawPict) {
		fail("East Asian Wide pictographs without Emoji_Presentation are %v, the spec lists %v", eawPict, listed)
	}
	if !contains(amb, 0xFFFD) {
		fail("U+FFFD (an invalid byte) must be in the ambiguous table")
	}
	for n, r := range standIns {
		if len(string(r)) != n || contains(amb, r) || contains(joiner, r) || contains(zero, r) || r == 0x2E3A || r == 0x2E3B {
			fail("stand-in %U for length %d is not an Other code point uniseg measures without the global", r, n)
		}
	}

	version := "unknown"
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, d := range bi.Deps {
			if d.Path == "github.com/rivo/uniseg" {
				version = d.Version
			}
		}
	}

	var b bytes.Buffer
	fmt.Fprintf(&b, "// Code generated by gen.go from github.com/rivo/uniseg %s (Unicode 15.0.0). DO NOT EDIT.\n\n", version)
	b.WriteString("package uniwidth\n\n")
	table := func(name, doc string, list []span) {
		fmt.Fprintf(&b, "// %s %s\nvar %s = []span{\n", name, doc, name)
		for i, s := range list {
			if i%4 == 0 {
				b.WriteString("\t")
			}
			fmt.Fprintf(&b, "{0x%04X, 0x%04X},", s.lo, s.hi)
			if i%4 == 3 || i == len(list)-1 {
				b.WriteString("\n")
			} else {
				b.WriteString(" ")
			}
		}
		b.WriteString("}\n\n")
	}
	table("zeroWidth", "holds the code points of width 0: Grapheme_Cluster_Break Control, CR,\n// LF, Extend, or ZWJ (rw rule 1).", zero)
	table("doubleWidth", "holds the code points of width 2: Regional_Indicator,\n// Extended_Pictographic with Emoji_Presentation or with East_Asian_Width\n// W or F, U+2E3A, U+2E3B, and East_Asian_Width W or F (rw rules 2 to 5).", wide)
	table("pictographic", "holds the Extended_Pictographic code points, whose clusters take\n// their width from a variation selector (W rule 1).", pict)
	table("ambiguous", "holds the code points whose uniseg width reads\n// uniseg.EastAsianAmbiguousWidth: East_Asian_Width A with\n// Grapheme_Cluster_Break Other. Next hands uniseg a stand-in instead.", amb)
	table("joiner", "holds the code points whose Grapheme_Cluster_Break is neither Other\n// nor Control, CR, or LF: the only ones a cluster boundary can depend on.", joiner)
	table("follower", "holds the code points of Grapheme_Cluster_Break Extend, ZWJ, or\n// SpacingMark, which join the cluster before them (GB9, GB9a).", follower)
	src, err := format.Source(b.Bytes())
	if err != nil {
		fail("%v", err)
	}
	if err := os.WriteFile("tables.go", src, 0o644); err != nil {
		fail("%v", err)
	}
}

func contains(list []span, r rune) bool {
	for _, s := range list {
		if r >= s.lo && r <= s.hi {
			return true
		}
	}
	return false
}
