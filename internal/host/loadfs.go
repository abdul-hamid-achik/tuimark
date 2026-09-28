package host

import (
	"fmt"
	iofs "io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/abdul-hamid-achik/tuimark/internal/parse"
)

// styleResolver resolves and reads a <style src> for loadStyles (SPEC
// v0.3 §18.1): the one difference between Load (an OS directory, where a
// relative src climbs above it with .. and any absolute src is used as
// given) and LoadFS (an fs.FS, where an absolute src, or one whose
// joined path climbs above the root, is a diagnostic instead of a read).
type styleResolver struct {
	// resolve joins a relative src against the document's directory and
	// reports whether it must be refused as outside the tree.
	resolve func(src string) (resolved string, outside bool)
	read    func(resolved string) ([]byte, error)
	// canon is the resolved path's identity for the include-cycle and
	// self-reference checks.
	canon func(resolved string) string
	// base is the display name a diagnostic and the stylesheet's own
	// File use (SPEC: "diagnostics name ... by their base names").
	base func(resolved string) string
	// self is this document's own canon key ("" for Parse, which has no
	// path to compare a src against).
	self string
}

// diskResolver resolves a <style src> against an OS directory, exactly as
// Load has always: a relative src joins dir; an absolute one is used as
// given; neither is ever "outside the tree" (SPEC v0.3 §18.1 decision 1:
// "Load keeps its resolution").
func diskResolver(dir, selfPath string) styleResolver {
	self := ""
	if selfPath != "" {
		self, _ = filepath.Abs(selfPath)
	}
	return styleResolver{
		resolve: func(src string) (string, bool) {
			p := src
			if !filepath.IsAbs(p) {
				p = filepath.Join(dir, p)
			}
			return p, false
		},
		read:  os.ReadFile,
		canon: func(p string) string { abs, _ := filepath.Abs(p); return abs },
		base:  filepath.Base,
		self:  self,
	}
}

// fsResolver resolves a <style src> inside fsys against a slash-separated
// dir (SPEC v0.3 §18.1): path.Join(path.Dir(name), src); an absolute src,
// or one whose joined path climbs above fsys's root, is "outside".
func fsResolver(fsys iofs.FS, dir, name string) styleResolver {
	return styleResolver{
		resolve: func(src string) (string, bool) {
			if path.IsAbs(src) {
				return src, true
			}
			joined := path.Join(dir, src)
			if joined == ".." || strings.HasPrefix(joined, "../") {
				return joined, true
			}
			return joined, false
		},
		read:  func(p string) ([]byte, error) { return iofs.ReadFile(fsys, p) },
		canon: path.Clean,
		base:  path.Base,
		self:  path.Clean(name),
	}
}

// LoadFS reads a .tui document named name from fsys and is otherwise Load
// (SPEC v0.3 §18.1): name must satisfy fs.ValidPath, or the error wraps
// fs.ErrInvalid; a read error is returned as an I/O error, like Load's. A
// relative <style src> resolves inside fsys as path.Join(path.Dir(name),
// src); an absolute src, or one whose joined path climbs above the root,
// is V006 "style src=... is outside the file system" instead of an I/O
// error, so the document still loads; any other src failure (missing, a
// .tui, included twice) is V006 as for Load. Diagnostics name the
// document and its stylesheets by their base names, as Load does.
func LoadFS(fsys iofs.FS, name string) (*App, error) {
	if !iofs.ValidPath(name) {
		return nil, fmt.Errorf("tuimark: LoadFS %q: %w", name, iofs.ErrInvalid)
	}
	src, err := iofs.ReadFile(fsys, name)
	if err != nil {
		return nil, err
	}
	return newAppFS(fsys, name, src), nil
}

func newAppFS(fsys iofs.FS, name string, src []byte) *App {
	dir := path.Dir(name)
	a := blankApp()
	a.dir, a.file = dir, filepath.Base(name)
	a.doc = parse.Parse(src, name)
	a.static = append(a.static, a.doc.Diags...)
	a.loadStyles(fsResolver(fsys, dir, name))
	return a
}
