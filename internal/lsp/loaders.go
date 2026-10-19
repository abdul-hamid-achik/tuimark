package lsp

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"

	"github.com/abdul-hamid-achik/tuimark/internal/host"
	"github.com/abdul-hamid-achik/tuimark/internal/ir"
	"github.com/abdul-hamid-achik/tuimark/internal/parse"
)

// parsedDoc is a .tui that loads a stylesheet, parsed and not checked.
type parsedDoc struct {
	path string
	text string
	doc  *parse.Document
}

// readText is the text of the file at path: its open buffer, else the file
// when it is a regular file of a bounded size (host.ReadRegular); any other
// path counts as absent.
func readText(sn *snapshot, path string) (string, bool) {
	if t, ok := sn.docText(path); ok {
		return t, true
	}
	raw, err := host.ReadRegular(path, 0)
	if err != nil {
		return "", false
	}
	return string(raw), true
}

// parsedLoaders are the .tui files that load the stylesheet at sheet: each
// open one, and each other .tui in its directory (SPEC §15.12
// "Diagnostics" and "Completion": the documents that load it).
func parsedLoaders(sn *snapshot, sheet string) []parsedDoc {
	var out []parsedDoc
	seen := map[string]bool{}
	add := func(path, text string) {
		if seen[path] {
			return
		}
		seen[path] = true
		doc := parse.Parse([]byte(text), path)
		if contains(loadedPaths(path, doc), sheet) {
			out = append(out, parsedDoc{path, text, doc})
		}
	}
	var open []string
	for _, d := range sn.byURI {
		if d.kind == kindTUI && d.path != "" {
			open = append(open, d.path)
		}
	}
	sort.Strings(open)
	for _, p := range open {
		add(p, sn.open[p])
	}
	entries, _ := os.ReadDir(filepath.Dir(sheet))
	for _, e := range entries {
		if e.IsDir() || kindOf("", e.Name()) != kindTUI {
			continue
		}
		p := filepath.Join(filepath.Dir(sheet), e.Name())
		if text, ok := readText(sn, p); ok {
			add(p, text)
		}
	}
	return out
}

// loadersVersion is the newest document version among docs, and the
// current language (4, SPEC v0.4b §5.1) when there are none.
func loadersVersion(docs []parsedDoc) int {
	if len(docs) == 0 {
		return int(ir.LangCurrent)
	}
	v := 1
	for _, d := range docs {
		v = max(v, docVersion(d.doc))
	}
	return v
}

var (
	idRE       = regexp.MustCompile(`\bid\s*=\s*"([^"]+)"`)
	classAttrR = regexp.MustCompile(`\bclass\s*=\s*"([^"]*)"`)
	guardRE    = regexp.MustCompile(`\bclass:([a-z_][a-z0-9_-]*)\s*=`)
	tokenDecl  = regexp.MustCompile(`--([A-Za-z_][A-Za-z0-9_-]*)\s*:`)
	wordSplit  = regexp.MustCompile(`\s+`)
	styleSrcRE = regexp.MustCompile(`<style\b[^>]*?\bsrc\s*=\s*"([^"]+)"`)
)

// idsOf are the ids written in a .tui, sorted, found in the text so they
// are there while the document is not yet well-formed.
func idsOf(text string) []string {
	return uniqueSorted(submatches(idRE, text))
}

// classesOf are the class names written in a .tui (class="a b" and
// class:NAME guards), sorted.
func classesOf(text string) []string {
	var out []string
	for _, v := range submatches(classAttrR, text) {
		for _, c := range wordSplit.Split(v, -1) {
			if c != "" {
				out = append(out, c)
			}
		}
	}
	return uniqueSorted(append(out, submatches(guardRE, text)...))
}

// tokensOf are the --token names a stylesheet declares.
func tokensOf(text string) []string {
	return uniqueSorted(submatches(tokenDecl, text))
}

func submatches(re *regexp.Regexp, text string) []string {
	var out []string
	for _, m := range re.FindAllStringSubmatch(text, -1) {
		out = append(out, m[1])
	}
	return out
}

func uniqueSorted(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

// walkNodes calls fn for every element of the document, in document order.
func walkNodes(doc *parse.Document, fn func(*ir.Node)) {
	var walk func(n *ir.Node)
	walk = func(n *ir.Node) {
		if n == nil {
			return
		}
		fn(n)
		for _, c := range n.Children {
			walk(c)
		}
	}
	if doc.Root != nil {
		walk(doc.Root)
		return
	}
	for _, sc := range doc.Screens {
		walk(sc)
	}
}

// hasClass reports whether the element has the class, written or guarded.
func hasClass(n *ir.Node, name string) bool {
	for _, c := range n.Classes {
		if c == name {
			return true
		}
	}
	for _, g := range n.ClassGuards {
		if g.Name == name {
			return true
		}
	}
	return false
}

// docVersionOf is the version of the .tui text without parsing it, as the
// parser reads it: the version attribute of its <tui> when it is 1, 2, or
// 3; the current language, 4, when the <tui> has no version (SPEC v0.4b
// §5.1, §15.12), and for a text with no root yet; 1 for any other value
// (version="4" included) and for the spike root <app>.
var (
	tuiOpenRE  = regexp.MustCompile(`<tui\b[^>]*`)
	appOpenRE  = regexp.MustCompile(`<app\b`)
	versionRE  = regexp.MustCompile(`\bversion\s*=\s*"([^"]*)"`)
	versionAny = regexp.MustCompile(`\bversion\s*=`)
)

func docVersionOf(text string) int {
	open := tuiOpenRE.FindString(text)
	if open == "" && appOpenRE.MatchString(text) {
		return 1
	}
	if m := versionRE.FindStringSubmatch(open); m != nil {
		if v, err := strconv.Atoi(m[1]); err == nil && v >= 1 && v <= 3 && m[1] == strconv.Itoa(v) {
			return v
		}
		return 1
	}
	if versionAny.MatchString(open) {
		// version= being typed, with no closing quote yet.
		return 1
	}
	return int(ir.LangCurrent)
}

// styleSrcsOf are the stylesheets a .tui text names, found in the text so
// they are there while the document is not yet well-formed.
func styleSrcsOf(path, text string) []string {
	var out []string
	for _, src := range submatches(styleSrcRE, text) {
		out = append(out, resolveSrc(path, src))
	}
	return out
}
