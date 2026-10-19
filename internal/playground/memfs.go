package playground

import (
	"io"
	"io/fs"
	"path"
	"sort"
	"strings"
	"time"
)

// memFS is a read-only in-memory fs.FS over a map of slash-separated
// paths to file contents. It is the file system tuimark.LoadFS loads a
// playground document from: <style src> resolves inside it, so the kit's
// "../theme.tcss" works when the map holds kit/theme.tcss.
type memFS map[string]string

// cleanKey normalizes a client-supplied path: a leading "./" or "/" is
// dropped, then the result must satisfy fs.ValidPath (no "..", no empty
// element). ok is false otherwise.
func cleanKey(p string) (string, bool) {
	p = strings.TrimPrefix(p, "./")
	p = strings.TrimLeft(p, "/")
	if p == "" || !fs.ValidPath(p) || p == "." {
		return "", false
	}
	return p, true
}

func (m memFS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	if c, ok := m[name]; ok {
		return &memFile{name: path.Base(name), r: strings.NewReader(c), size: int64(len(c))}, nil
	}
	// A directory exists when some file lives below it.
	prefix := name + "/"
	if name == "." {
		prefix = ""
	}
	seen := map[string]bool{}
	var ents []fs.DirEntry
	for p := range m {
		if !strings.HasPrefix(p, prefix) {
			continue
		}
		rest := p[len(prefix):]
		first, _, nested := strings.Cut(rest, "/")
		if seen[first] {
			continue
		}
		seen[first] = true
		e := memEntry{name: first, dir: nested}
		if !nested {
			e.size = int64(len(m[p]))
		}
		ents = append(ents, e)
	}
	if len(ents) == 0 && name != "." {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	sort.Slice(ents, func(i, j int) bool { return ents[i].Name() < ents[j].Name() })
	return &memDir{name: path.Base(name), ents: ents}, nil
}

// ReadFile is fs.ReadFileFS, so LoadFS and <style src> skip Open.
func (m memFS) ReadFile(name string) ([]byte, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "readfile", Path: name, Err: fs.ErrInvalid}
	}
	c, ok := m[name]
	if !ok {
		return nil, &fs.PathError{Op: "readfile", Path: name, Err: fs.ErrNotExist}
	}
	return []byte(c), nil
}

type memFile struct {
	name string
	r    *strings.Reader
	size int64
}

func (f *memFile) Stat() (fs.FileInfo, error) { return memInfo{name: f.name, size: f.size}, nil }
func (f *memFile) Read(b []byte) (int, error) { return f.r.Read(b) }
func (f *memFile) Close() error               { return nil }

type memDir struct {
	name string
	ents []fs.DirEntry
	off  int
}

func (d *memDir) Stat() (fs.FileInfo, error) { return memInfo{name: d.name, dir: true}, nil }
func (d *memDir) Read([]byte) (int, error) {
	return 0, &fs.PathError{Op: "read", Path: d.name, Err: fs.ErrInvalid}
}
func (d *memDir) Close() error { return nil }
func (d *memDir) ReadDir(n int) ([]fs.DirEntry, error) {
	rest := d.ents[d.off:]
	if n <= 0 {
		d.off = len(d.ents)
		return rest, nil
	}
	if len(rest) == 0 {
		return nil, io.EOF
	}
	if n > len(rest) {
		n = len(rest)
	}
	d.off += n
	return rest[:n], nil
}

type memInfo struct {
	name string
	size int64
	dir  bool
}

func (i memInfo) Name() string { return i.name }
func (i memInfo) Size() int64  { return i.size }
func (i memInfo) Mode() fs.FileMode {
	if i.dir {
		return fs.ModeDir | 0o555
	}
	return 0o444
}
func (i memInfo) ModTime() time.Time { return time.Time{} }
func (i memInfo) IsDir() bool        { return i.dir }
func (i memInfo) Sys() any           { return nil }

type memEntry struct {
	name string
	dir  bool // the entry has a "/" below it
	size int64
}

func (e memEntry) Name() string { return e.name }
func (e memEntry) IsDir() bool  { return e.dir }
func (e memEntry) Type() fs.FileMode {
	if e.IsDir() {
		return fs.ModeDir
	}
	return 0
}
func (e memEntry) Info() (fs.FileInfo, error) {
	return memInfo{name: e.name, dir: e.IsDir(), size: e.size}, nil
}
