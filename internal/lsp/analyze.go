package lsp

import (
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/abdul-hamid-achik/tuimark/internal/host"
	"github.com/abdul-hamid-achik/tuimark/internal/ir"
	"github.com/abdul-hamid-achik/tuimark/internal/parse"
)

// sampleName is the data file a .tui is checked against when it sits in
// the document's directory (SPEC §15.12 "The data").
const sampleName = "sample.json"

// untitledPath stands in for the path of a buffer that is not a file, so
// the loader has a name to give diagnostics; a <style src> of such a
// document resolves against the server's working directory.
const untitledPath = "untitled.tui"

// analysis is what `validate` says about one .tui, with the texts the
// positions refer to.
type analysis struct {
	path   string // the document's path
	text   string
	sheets []host.Source // the stylesheets that were read
	// loadPaths is every stylesheet the document names, read or not.
	loadPaths []string
	diags     ir.Diags
	hasData   bool
	data      any // the decoded sample.json, nil without one
	version   int // 1, 2, 3, or 4 (the current language)
}

// LSP severities (DiagnosticSeverity).
const (
	severityError   = 1
	severityWarning = 2
)

// lspDiagnostic is an LSP Diagnostic.
type lspDiagnostic struct {
	Range    Range  `json:"range"`
	Severity int    `json:"severity"`
	Code     string `json:"code"`
	Source   string `json:"source"`
	Message  string `json:"message"`
}

// analyze runs validate's checks on the document at path whose text is
// text: the same loader (reading open buffers for its stylesheets), the
// sibling sample.json as --data, the same themes (a theme="auto" document
// under both), and the same filter when there is no data.
func analyze(sn *snapshot, path, text string) *analysis {
	if path == "" {
		path = untitledPath
	}
	app, sheets := host.LoadBuffers(path, []byte(text), func(abs string) ([]byte, bool) {
		t, ok := sn.docText(abs)
		return []byte(t), ok
	})
	a := &analysis{path: path, text: text, sheets: sheets, version: docVersion(app.Document()), loadPaths: loadedPaths(path, app.Document())}
	a.data, a.hasData = readSample(sn, path)
	if a.hasData {
		if err := app.Bind("", a.data); err != nil {
			a.data, a.hasData = nil, false
		}
	}
	diags := app.Validate()
	if doc := app.Document(); doc.FragmentRoot {
		// A file whose root is <fragment> is no document: it is checked
		// on its own, as `tuimark validate` checks it, with no V005 (SPEC
		// v0.4b §6.18.2, §15.12). Routing its diagnostics through the
		// documents that include it is F4's.
		diags = parse.FragmentDiags(doc)
	}
	if !a.hasData {
		// Without data every bind path is "missing": keep the data-free
		// checks, as validate does (B007 is static).
		var kept ir.Diags
		for _, d := range diags {
			switch d.Code {
			case "B001", "B002", "B003", "B008", "B009", "B010", "B011", "B012":
				continue
			}
			kept = append(kept, d)
		}
		diags = kept
	}
	a.diags = diags
	return a
}

// docVersion is the document version the vocabulary is filtered by: 4 for
// a <tui> without version (the current language, SPEC v0.4b §5.1), 3 and
// 2 for those documents, else 1 (any other version is read as 1).
func docVersion(d *parse.Document) int {
	switch {
	case d.FragmentRoot:
		// A fragment is always the current language (SPEC v0.4b §6.18.2).
		return int(ir.LangCurrent)
	case d.Lang.V4():
		return int(ir.LangCurrent)
	case d.V3:
		return 3
	case d.V2:
		return 2
	}
	return 1
}

// readSample reads and decodes the sample.json next to the .tui at path:
// the open buffer when it is open, else the file. Absent or not valid
// JSON, the document is checked without data.
func readSample(sn *snapshot, path string) (any, bool) {
	if path == untitledPath {
		return nil, false
	}
	p := filepath.Join(filepath.Dir(path), sampleName)
	text, ok := sn.docText(p)
	if !ok {
		raw, err := host.ReadRegular(p, 0)
		if err != nil {
			return nil, false
		}
		text = string(raw)
	}
	var v any
	if err := json.Unmarshal([]byte(text), &v); err != nil {
		return nil, false
	}
	return v, true
}

// loadedPaths is the absolute path of every stylesheet the document names,
// whether or not it could be read.
func loadedPaths(path string, doc *parse.Document) []string {
	var out []string
	for _, s := range doc.Styles {
		if s.Src != "" {
			out = append(out, resolveSrc(path, s.Src))
		}
	}
	return out
}

// resolveSrc resolves a <style src> as Load does: joined to the
// document's directory (.. included), an absolute one as given.
func resolveSrc(docPath, src string) string {
	p := src
	if !filepath.IsAbs(p) {
		p = filepath.Join(filepath.Dir(docPath), p)
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return filepath.Clean(p)
	}
	return abs
}

// loads reports whether the analysis read the stylesheet at sheet.
func (a *analysis) loads(sheet string) bool {
	for _, s := range a.sheets {
		if s.Path == sheet {
			return true
		}
	}
	return false
}

// sheetText is the text of a stylesheet the analysis read.
func (a *analysis) sheetText(sheet string) string {
	for _, s := range a.sheets {
		if s.Path == sheet {
			return string(s.Text)
		}
	}
	return ""
}

// route returns the diagnostics whose file is the document (inDoc) or the
// stylesheet at sheet. The loader names a stylesheet by its absolute path
// and the document by its base name, so the two cannot be mistaken.
func (a *analysis) route(sheet string, inDoc bool) []lspDiagnostic {
	var out []lspDiagnostic
	var lines []string
	for _, d := range a.diags {
		if inDoc && filepath.IsAbs(d.File) || !inDoc && d.File != sheet {
			continue
		}
		if lines == nil {
			if inDoc {
				lines = splitLines(a.text)
			} else {
				lines = splitLines(a.sheetText(sheet))
			}
		}
		out = append(out, convertDiag(lines, d))
	}
	return out
}

// convertDiag is the LSP form of a diagnostic: validate's message and
// severity, and a range from its position to the end of the token there,
// or of the line (SPEC §15.12 "Ranges").
func convertDiag(lines []string, d ir.Diagnostic) lspDiagnostic {
	sev := severityError
	if d.Severity == ir.Warning {
		sev = severityWarning
	}
	start := fromDiag(lines, d.Line, d.Col)
	end := start
	if d.Line >= 1 && d.Line <= len(lines) {
		end.Character = tokenEnd(lines[start.Line], start.Character)
	}
	return lspDiagnostic{Range: Range{Start: start, End: end}, Severity: sev, Code: d.Code, Source: "tuimark", Message: d.Msg}
}

// tokenEnd is the UTF-16 offset where the token starting at offset start
// of line ends: a leading "<" or "</" and then the run of name characters
// (letters, digits, and _ - : . # $ @ %), or, when none starts there, the
// end of the line.
func tokenEnd(line string, start int) int {
	units := 0
	runes := []rune(line)
	i := 0
	for i < len(runes) && units < start {
		units += runeUnits(runes[i])
		i++
	}
	j := i
	for j < len(runes) && (runes[j] == '<' || runes[j] == '/') {
		j++
	}
	k := j
	for k < len(runes) && isWordRune(runes[k]) {
		k++
	}
	for k > j && (runes[k-1] == ':' || runes[k-1] == '.') { // a property's ":" is not part of it
		k--
	}
	if k == j {
		k = len(runes)
	}
	end := start
	for _, r := range runes[i:k] {
		end += runeUnits(r)
	}
	return end
}

// isWordRune is a character of a tag, attribute, property, or keyword name
// and of the #id, .class, $token, and :pseudo that begin with a sigil.
func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("_-:.#$@%", r)
}

// dedupe drops a diagnostic whose code, position, and message an earlier
// one already has, keeping the order of first appearance.
func dedupe(in []lspDiagnostic) []lspDiagnostic {
	type key struct {
		code, msg string
		pos       Position
	}
	seen := map[key]bool{}
	out := make([]lspDiagnostic, 0, len(in))
	for _, d := range in {
		k := key{d.Code, d.Message, d.Range.Start}
		if !seen[k] {
			seen[k] = true
			out = append(out, d)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i].Range.Start, out[j].Range.Start
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.Character < b.Character
	})
	return out
}
