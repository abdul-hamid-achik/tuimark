package host

import (
	"errors"
	"fmt"
	"io"
	iofs "io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/abdul-hamid-achik/tuimark/internal/parse"
)

// styleResolver resolves and reads the files a document names, a <style
// src> for loadStyles (SPEC v0.3 §18.1) and an <include src> for the
// parser (SPEC v0.4b §6.18.2): the one difference between Load (an OS
// directory, where a relative src climbs above it with .. and any absolute
// src is used as given) and LoadFS (an fs.FS, where an absolute src, or one
// whose joined path climbs above the root, is a diagnostic instead of a
// read).
type styleResolver struct {
	// resolve joins a relative src against the directory of the file at
	// path from ("" is the document) and reports whether it must be
	// refused as outside the tree.
	resolve func(from, src string) (resolved string, outside bool)
	read    func(resolved string) ([]byte, error)
	// canon is the resolved path's identity for the include-cycle and
	// self-reference checks.
	canon func(resolved string) string
	// base is the display name a diagnostic and the stylesheet's own
	// File use (SPEC: "diagnostics name ... by their base names").
	base func(resolved string) string
	// name is the display name of a file relative to the document's
	// directory, with "/" separators: a fragment's, and a stylesheet a
	// fragment loads (SPEC v0.4b §6.18.6).
	name func(resolved string) string
	// readFile reads an included fragment; nil means read.
	readFile func(resolved string) ([]byte, error)
	// same reports whether two resolved paths are the same file on disk;
	// nil where the file system has no file identity (ADR 0034).
	same func(a, b string) bool
	// selfPath is this document's own resolved path ("" for Parse).
	selfPath string
	// self is this document's own canon key ("" for Parse, which has no
	// path to compare a src against).
	self string
}

// loader is the parser's view of the same file system.
func (r styleResolver) loader() *parse.Loader {
	read := r.read
	if r.readFile != nil {
		read = r.readFile
	}
	return &parse.Loader{Join: r.resolve, Read: read, Canon: r.canon, Name: r.name, Self: r.self, SelfPath: r.selfPath, Same: r.same}
}

// diskResolver resolves a <style src> against an OS directory, exactly as
// Load has always: a relative src joins dir; an absolute one is used as
// given; neither is ever "outside the tree" (SPEC v0.3 §18.1 decision 1:
// "Load keeps its resolution"). A file included from a fragment resolves
// against that fragment's directory.
func diskResolver(dir, selfPath string) styleResolver {
	self := ""
	if selfPath != "" {
		self, _ = filepath.Abs(selfPath)
	}
	absDir, _ := filepath.Abs(dir)
	stat := statCache(os.Stat)
	return styleResolver{
		resolve: func(from, src string) (string, bool) {
			p := src
			if !filepath.IsAbs(p) {
				d := dir
				if from != "" {
					d = filepath.Dir(from)
				}
				p = filepath.Join(d, p)
			}
			return p, false
		},
		read: os.ReadFile,
		// An included fragment is read with a bound: nothing but a regular
		// file, and no more than MaxReadBytes (a FIFO or /dev/zero named by
		// an include would otherwise hang or exhaust memory).
		readFile: func(p string) ([]byte, error) {
			data, err := ReadRegular(p, 0)
			return data, includeReadErr(err)
		},
		canon:    func(p string) string { abs, _ := filepath.Abs(p); return abs },
		selfPath: selfPath,
		same: func(a, b string) bool {
			fa, err := stat(a)
			if err != nil {
				return false
			}
			fb, err := stat(b)
			return err == nil && os.SameFile(fa, fb)
		},
		base: filepath.Base,
		name: func(p string) string {
			abs, err := filepath.Abs(p)
			if err != nil {
				return filepath.ToSlash(p)
			}
			rel, err := filepath.Rel(absDir, abs)
			if err != nil {
				return filepath.ToSlash(abs)
			}
			return filepath.ToSlash(rel)
		},
		self: self,
	}
}

// fsResolver resolves a <style src> inside fsys against a slash-separated
// dir (SPEC v0.3 §18.1): path.Join(path.Dir(name), src); an absolute src,
// or one whose joined path climbs above fsys's root, is "outside".
func fsResolver(fsys iofs.FS, dir, name string) styleResolver {
	stat := statCache(func(p string) (iofs.FileInfo, error) { return iofs.Stat(fsys, p) })
	return styleResolver{
		resolve: func(from, src string) (string, bool) {
			if path.IsAbs(src) {
				return src, true
			}
			d := dir
			if from != "" {
				d = path.Dir(from)
			}
			joined := path.Join(d, src)
			if joined == ".." || strings.HasPrefix(joined, "../") {
				return joined, true
			}
			return joined, false
		},
		read: func(p string) ([]byte, error) { return iofs.ReadFile(fsys, p) },
		readFile: func(p string) ([]byte, error) {
			data, err := readFSRegular(fsys, p)
			return data, includeReadErr(err)
		},
		canon:    path.Clean,
		selfPath: name,
		// os.DirFS answers with os file information, which SameFile
		// compares; an fs.FS without file identity (fstest.MapFS, the
		// playground's) never is the same file but by path.
		same: func(a, b string) bool {
			fa, err := stat(a)
			if err != nil {
				return false
			}
			fb, err := stat(b)
			return err == nil && os.SameFile(fa, fb)
		},
		base: path.Base,
		name: func(p string) string { return slashRel(dir, p) },
		self: path.Clean(name),
	}
}

// includeReadErr words a refusal of ReadRegular for an include's V006 "cannot
// be read": the reason alone, without the path the message already names.
func includeReadErr(err error) error {
	switch {
	case errors.Is(err, ErrNotRegular):
		return errors.New("not a regular file")
	case errors.Is(err, ErrTooLarge):
		return fmt.Errorf("file too large (over %d bytes)", MaxReadBytes)
	}
	return err
}

// readFSRegular is ReadRegular for a file inside an fs.FS: it refuses a file
// that is not regular, and one of more than MaxReadBytes.
func readFSRegular(fsys iofs.FS, name string) ([]byte, error) {
	fi, err := iofs.Stat(fsys, name)
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("%s: %w", name, ErrNotRegular)
	}
	f, err := fsys.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, MaxReadBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > MaxReadBytes {
		return nil, fmt.Errorf("%s: %w (over %d bytes)", name, ErrTooLarge, int64(MaxReadBytes))
	}
	return data, nil
}

// statCache memoizes stat for one load: the include cycle check asks about
// the same few files for every include of an expansion.
func statCache(stat func(string) (iofs.FileInfo, error)) func(string) (iofs.FileInfo, error) {
	type res struct {
		fi  iofs.FileInfo
		err error
	}
	m := map[string]res{}
	return func(p string) (iofs.FileInfo, error) {
		if r, ok := m[p]; ok {
			return r.fi, r.err
		}
		fi, err := stat(p)
		m[p] = res{fi, err}
		return fi, err
	}
}

// slashRel is target relative to base, both cleaned slash-separated paths
// inside one fs.FS: "../shared/foot.tui" from "ui" to "shared/foot.tui".
func slashRel(base, target string) string {
	b := strings.Split(path.Clean(base), "/")
	t := strings.Split(path.Clean(target), "/")
	if len(b) == 1 && b[0] == "." {
		b = nil
	}
	i := 0
	for i < len(b) && i < len(t) && b[i] == t[i] {
		i++
	}
	out := strings.Repeat("../", len(b)-i) + strings.Join(t[i:], "/")
	if out == "" {
		return "."
	}
	return out
}

// LoadFS reads a .tui document named name from fsys and is otherwise Load
// (SPEC v0.3 §18.1): name must satisfy fs.ValidPath, or the error wraps
// fs.ErrInvalid; a read error is returned as an I/O error, like Load's. A
// relative <style src> resolves inside fsys as path.Join(path.Dir(name),
// src); an absolute src, or one whose joined path climbs above the root,
// is V006 "style src=... is outside the file system" instead of an I/O
// error, so the document still loads; any other src failure (missing, a
// .tui, included twice) is V006 as for Load. Diagnostics name the
// document and its stylesheets by their base names, as Load does. An
// <include src> resolves the same way (SPEC v0.4b §6.18.2), and the
// fragments are named by their path relative to the document's directory.
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
	res := fsResolver(fsys, dir, name)
	a.ldr = res.loader()
	a.doc = parse.ParseWith(src, name, a.ldr)
	a.static = append(a.static, a.doc.Diags...)
	a.loadStyles(res)
	return a
}
