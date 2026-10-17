package lsp

import "github.com/abdul-hamid-achik/tuimark/internal/parse"

// formatDocument answers textDocument/formatting (SPEC §15.12
// "Formatting"): for a .tui, one edit replacing the whole text with what
// `tuimark fmt` prints; no edit when the text is already canonical, and
// none for a document with a V005 (fmt cannot format what is not
// well-formed). A .tcss has no formatter.
func formatDocument(doc *document) []textEdit {
	edits := []textEdit{}
	if doc.kind != kindTUI {
		return edits
	}
	formatted, err := parse.Format([]byte(doc.text))
	if err != nil || formatted == doc.text {
		return edits
	}
	return append(edits, textEdit{
		Range:   Range{Start: Position{}, End: endPosition(doc.text)},
		NewText: formatted,
	})
}
