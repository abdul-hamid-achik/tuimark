package lsp

import (
	"os"
	"path/filepath"
	"testing"
)

// Every feature answers at every position of real documents, whatever the
// cursor is in, without panicking: a buffer being typed is any prefix of a
// document.
func TestFeaturesAtEveryOffset(t *testing.T) {
	root := filepath.Join("..", "..", "examples", "inbox")
	app, err := filepath.Abs(filepath.Join(root, "app.tui"))
	if err != nil {
		t.Fatal(err)
	}
	sheet := filepath.Join(filepath.Dir(app), "theme.tcss")
	appText, err := os.ReadFile(filepath.Join(root, "app.tui"))
	if err != nil {
		t.Fatal(err)
	}
	sheetText, err := os.ReadFile(filepath.Join(root, "theme.tcss"))
	if err != nil {
		t.Fatal(err)
	}
	wide := "<tui version=\"3\">\n  <screen id=\"日本😀\"><text class=\"x\">{a.b}</text>\n"
	s := newServer(Options{})
	s.afterFunc = (&fakeClock{}).afterFunc // nothing fires
	s.didOpen(pathToURI(app), "tuimark", string(appText), 1)
	s.didOpen(pathToURI(sheet), "tcss", string(sheetText), 1)
	s.didOpen("untitled:wide", "tuimark", wide, 1)
	sn := s.snapshot()
	for _, d := range sn.byURI {
		for off := 0; off <= len(d.text); off += 3 {
			pos := positionAt(d.text, off)
			s.complete(sn, d, pos)
			s.hover(sn, d, pos)
			s.definition(sn, d, pos)
		}
		// Every prefix is a document some editor holds at some moment.
		for end := 0; end <= len(d.text); end += 17 {
			c := *d
			c.text = d.text[:end]
			s.complete(sn, &c, endPosition(c.text))
			formatDocument(&c)
			analyze(sn, c.path, c.text)
		}
	}
}
