package host

import (
	"path/filepath"

	"github.com/abdul-hamid-achik/tuimark/internal/parse"
)

// Source is a stylesheet LoadBuffers read: its absolute path and the text
// that was parsed (an open buffer's, or the file's).
type Source struct {
	Path string
	Text []byte
}

// LoadBuffers is Load for an editor (SPEC v0.4a §15.12): the document is
// the text src of the file path, not what is on disk, and a <style src> is
// read from the open buffer open(abs) reports for it when there is one,
// else from disk. A src resolves exactly as Load resolves it (joined to
// the document's directory, .. included, an absolute one as given). An
// <include src> is read the same way (SPEC v0.4b §6.18.2): a fragment is
// the editor's text when it is open, else the file.
//
// One more difference from Load: a stylesheet's diagnostics name it by its
// absolute path, not its base name, so a caller can route them by file
// (two stylesheets called theme.tcss in different directories stay apart).
// The same holds for a fragment (SPEC v0.4b §6.18.6: the language server
// names every file by its absolute path). The document's own diagnostics
// keep the base name. The returned Sources are the stylesheets that were
// read, in document order.
func LoadBuffers(path string, src []byte, open func(abs string) ([]byte, bool)) (*App, []Source) {
	dir := filepath.Dir(path)
	res := diskResolver(dir, path)
	var read []Source
	buffer := func(p string) ([]byte, string, error) {
		abs, _ := filepath.Abs(p)
		text, ok := open(abs)
		if !ok {
			var err error
			if text, err = ReadRegular(p, 0); err != nil {
				return nil, abs, err
			}
		}
		return text, abs, nil
	}
	res.read = func(p string) ([]byte, error) {
		text, abs, err := buffer(p)
		if err != nil {
			return nil, err
		}
		read = append(read, Source{Path: abs, Text: text})
		return text, nil
	}
	res.readFile = func(p string) ([]byte, error) {
		text, _, err := buffer(p)
		return text, includeReadErr(err)
	}
	res.base = func(p string) string { abs, _ := filepath.Abs(p); return abs }
	res.name = res.base
	a := blankApp()
	a.path, a.dir, a.file = path, dir, filepath.Base(path)
	a.doc = parse.ParseWith(src, path, res.loader())
	a.static = append(a.static, a.doc.Diags...)
	a.loadStyles(res)
	return a, read
}
