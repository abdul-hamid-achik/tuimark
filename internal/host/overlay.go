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
// the document's directory, .. included, an absolute one as given).
//
// One more difference from Load: a stylesheet's diagnostics name it by its
// absolute path, not its base name, so a caller can route them by file
// (two stylesheets called theme.tcss in different directories stay apart).
// The document's own diagnostics keep the base name. The returned Sources
// are the stylesheets that were read, in document order.
func LoadBuffers(path string, src []byte, open func(abs string) ([]byte, bool)) (*App, []Source) {
	dir := filepath.Dir(path)
	res := diskResolver(dir, path)
	var read []Source
	res.read = func(p string) ([]byte, error) {
		abs, _ := filepath.Abs(p)
		text, ok := open(abs)
		if !ok {
			var err error
			if text, err = ReadRegular(p, 0); err != nil {
				return nil, err
			}
		}
		read = append(read, Source{Path: abs, Text: text})
		return text, nil
	}
	res.base = func(p string) string { abs, _ := filepath.Abs(p); return abs }
	a := blankApp()
	a.path, a.dir, a.file = path, dir, filepath.Base(path)
	a.doc = parse.Parse(src, path)
	a.static = append(a.static, a.doc.Diags...)
	a.loadStyles(res)
	return a, read
}
