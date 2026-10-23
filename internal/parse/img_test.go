package parse_test

import (
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/ir"
	"github.com/abdul-hamid-achik/tuimark/internal/parse"
)

// findImg walks the expanded tree for the first <img> element.
func findImg(n *ir.Node) *ir.Node {
	if n == nil {
		return nil
	}
	if n.Kind == "img" {
		return n
	}
	for _, c := range n.Children {
		if t := findImg(c); t != nil {
			return t
		}
	}
	return nil
}

func imgDiags(t *testing.T, src string) *parse.Document {
	t.Helper()
	doc := parse.Parse([]byte(src), "a.tui")
	if doc.Root == nil {
		t.Fatalf("no root: %v", doc.Diags)
	}
	return doc
}

// An <img> is self-closing with src, alt, width, and height; a data: URI
// is a src like any other, and src/alt are sanitized like text (V007).
func TestImgAttributesParsed(t *testing.T) {
	doc := imgDiags(t, `<tui><screen><img id="shot" src="img/tiny.webp" alt="a dot" width="20" height="10"/></screen></tui>`)
	if doc.Diags.HasErrors() {
		t.Fatalf("diags: %v", doc.Diags)
	}
	n := findImg(doc.Root)
	if n == nil {
		t.Fatal("no <img> in the document")
	}
	for attr, want := range map[string]string{"src": "img/tiny.webp", "alt": "a dot", "width": "20", "height": "10"} {
		if got := n.Attrs[attr]; got != want {
			t.Errorf("%s = %q, want %q", attr, got, want)
		}
	}
	if len(n.Hints) != 2 {
		t.Errorf("hints = %v, want width and height", n.Hints)
	}

	doc = imgDiags(t, `<tui><screen><img src="data:image/png;base64,aGVsbG8="/></screen></tui>`)
	if doc.Diags.HasErrors() {
		t.Fatalf("data URI diags: %v", doc.Diags)
	}
	if n := findImg(doc.Root); n.Attrs["src"] != "data:image/png;base64,aGVsbG8=" {
		t.Errorf("src = %q", n.Attrs["src"])
	}

	// A control character in src or alt is V007, stripped like text.
	doc = imgDiags(t, "<tui><screen><img src=\"a\x1b[0mb\" alt=\"x\x07y\"/></screen></tui>")
	if !doc.Diags.HasErrors() || doc.Diags[0].Code != "V007" {
		t.Fatalf("diags = %v, want V007", doc.Diags)
	}
	if n := findImg(doc.Root); n.Attrs["src"] != "ab" || n.Attrs["alt"] != "xy" {
		t.Errorf("sanitized src/alt = %q/%q, want ab/xy", n.Attrs["src"], n.Attrs["alt"])
	}
}

// src is required; content and element children are V013; width/height
// are cell counts (plain non-negative integers).
func TestImgStructureChecked(t *testing.T) {
	doc := imgDiags(t, `<tui><screen><img/></screen></tui>`)
	if !doc.Diags.HasErrors() || doc.Diags[0].Code != "V003" {
		t.Fatalf("no src: diags = %v, want V003", doc.Diags)
	}
	if !strings.Contains(doc.Diags[0].Msg, "src") {
		t.Errorf("V003 message = %q", doc.Diags[0].Msg)
	}

	doc = imgDiags(t, `<tui><screen><img src="a.png">words</img></screen></tui>`)
	if !doc.Diags.HasErrors() || doc.Diags[0].Code != "V013" {
		t.Fatalf("text content: diags = %v, want V013", doc.Diags)
	}

	doc = imgDiags(t, `<tui><screen><img src="a.png"><text>x</text></img></screen></tui>`)
	if !doc.Diags.HasErrors() || doc.Diags[0].Code != "V013" {
		t.Fatalf("element child: diags = %v, want V013", doc.Diags)
	}
	if !strings.Contains(doc.Diags[0].Msg, "<img>") {
		t.Errorf("V013 message = %q", doc.Diags[0].Msg)
	}

	for _, attr := range []string{"width", "height"} {
		doc = imgDiags(t, `<tui><screen><img src="a.png" `+attr+`="50%"/></screen></tui>`)
		if !doc.Diags.HasErrors() || doc.Diags[0].Code != "V003" {
			t.Fatalf("%s=%%: diags = %v, want V003", attr, doc.Diags)
		}
	}
	// 0 is a valid cell count (the box collapses).
	doc = imgDiags(t, `<tui><screen><img src="a.png" width="0" height="0"/></screen></tui>`)
	if doc.Diags.HasErrors() {
		t.Fatalf("0 sizes: %v", doc.Diags)
	}
}

// An <img> is current-language vocabulary: in a pinned document the tag
// is V001 with the hint, and its attributes are V002 there.
func TestImgNeedsCurrentLanguage(t *testing.T) {
	doc := imgDiags(t, `<tui version="3"><screen><img src="a.png"/></screen></tui>`)
	if !doc.Diags.HasErrors() || doc.Diags[0].Code != "V001" {
		t.Fatalf("diags = %v, want V001", doc.Diags)
	}
	if !strings.Contains(doc.Diags[0].Msg, "without version") {
		t.Errorf("V001 message = %q", doc.Diags[0].Msg)
	}
	// It takes no attribute of another tag, even in the current language.
	doc = imgDiags(t, `<tui><screen><img src="a.png" bind="x"/></screen></tui>`)
	if !doc.Diags.HasErrors() || doc.Diags[0].Code != "V002" {
		t.Fatalf("diags = %v, want V002", doc.Diags)
	}
}

// An <img>'s quality is auto (the default), photo, or blocks (v0.4d
// phase 3); any other value is V003, and no other tag takes it.
func TestImgQuality(t *testing.T) {
	for _, val := range []string{"auto", "photo", "blocks"} {
		doc := imgDiags(t, `<tui><screen><img src="a.png" quality="`+val+`"/></screen></tui>`)
		if doc.Diags.HasErrors() {
			t.Fatalf("quality=%q: %v", val, doc.Diags)
		}
		if got := findImg(doc.Root).Attrs["quality"]; got != val {
			t.Errorf("quality=%q parsed as %q", val, got)
		}
	}
	doc := imgDiags(t, `<tui><screen><img src="a.png" quality="high"/></screen></tui>`)
	if !doc.Diags.HasErrors() || doc.Diags[0].Code != "V003" {
		t.Fatalf("quality=high: diags = %v, want V003", doc.Diags)
	}
	if !strings.Contains(doc.Diags[0].Msg, "auto, photo, or blocks") {
		t.Errorf("V003 message = %q", doc.Diags[0].Msg)
	}
	// The keyword table of `tuimark catalog` names the same three.
	if kws := parse.AttrKeywords("img", "quality"); len(kws) != 3 ||
		kws[0] != (parse.AttrKeyword{"auto", 4}) || kws[1] != (parse.AttrKeyword{"photo", 4}) || kws[2] != (parse.AttrKeyword{"blocks", 4}) {
		t.Errorf("img.quality keywords = %+v", kws)
	}
}
