package lsp

import (
	"net/url"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// fileKind is what a document is for the server: a .tui view, a .tcss
// stylesheet, or something it only keeps text of (a sample.json).
type fileKind int

const (
	kindOther fileKind = iota
	kindTUI
	kindTCSS
)

// document is an open buffer.
type document struct {
	uri     string
	path    string // the absolute file path; "" for a URI that is not a file
	kind    fileKind
	text    string
	version int
	// loads is the absolute paths of the files whose diagnostics the last
	// publish of a .tui found it moves (Server.noteLoads): the stylesheets
	// and fragments it read (and those it tried to), or, for a fragment,
	// the other files the documents that read it read. A change finds
	// through it the files to publish again: the ones it loads, and the
	// open .tui files whose loads name it.
	loads []string
}

var driveRE = regexp.MustCompile(`^/[A-Za-z]:`)

// uriToPath is the file path of a file: URI, and false for any other
// scheme (an untitled: buffer has no place on disk).
func uriToPath(uri string) (string, bool) {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "file" {
		return "", false
	}
	p := u.Path
	if runtime.GOOS == "windows" && driveRE.MatchString(p) {
		p = p[1:]
	}
	return filepath.Clean(filepath.FromSlash(p)), true
}

// pathToURI is the file: URI of an absolute path.
func pathToURI(p string) string {
	p = filepath.ToSlash(p)
	if driveRE.MatchString("/" + p) {
		p = "/" + p
	}
	return (&url.URL{Scheme: "file", Path: p}).String()
}

// kindOf classifies a document by its language id when the client gave
// one, else by its extension.
func kindOf(languageID, name string) fileKind {
	switch languageID {
	case "tuimark":
		return kindTUI
	case "tcss":
		return kindTCSS
	}
	switch strings.ToLower(filepath.Ext(name)) {
	case ".tui":
		return kindTUI
	case ".tcss":
		return kindTCSS
	}
	return kindOther
}

// snapshot is the open buffers at one moment, so an analysis can run
// without holding the server's lock.
type snapshot struct {
	gen   int
	byURI map[string]*document // copies
	open  map[string]string    // file path -> text
}

// docText is the text of the buffer open for path, if any.
func (sn *snapshot) docText(path string) (string, bool) {
	t, ok := sn.open[path]
	return t, ok
}
