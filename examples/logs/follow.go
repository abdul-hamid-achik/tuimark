package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

// fileSource tails a real file (-follow FILE): each Next stats the file,
// reads whatever was appended since the last call, and turns every
// complete line into an entry with a level guessed from its text.
type fileSource struct {
	path  string
	name  string
	off   int64
	carry []byte // the unfinished last line of the previous read
	now   func() time.Time
}

const (
	tailBytes = 64 << 10 // how much of the file's end the first view shows
	readMax   = 1 << 20  // the most one tick reads
)

// newFileSource opens path (an error here fails the start, not a tick) and
// returns the source positioned at the end of the file, with the last
// tailBytes of it as the opening backlog.
func newFileSource(path string, now func() time.Time) (*fileSource, []entry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, nil, err
	}
	if st.IsDir() {
		return nil, nil, fmt.Errorf("%s is a directory", path)
	}
	fs := &fileSource{path: path, name: filepath.Base(path), now: now}
	start := st.Size() - tailBytes
	if start < 0 {
		start = 0
	}
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return nil, nil, err
	}
	raw, err := io.ReadAll(f)
	if err != nil {
		return nil, nil, err
	}
	fs.off = start + int64(len(raw))
	if start > 0 { // the first line is cut in the middle: drop it
		if i := bytes.IndexByte(raw, '\n'); i >= 0 {
			raw = raw[i+1:]
		} else {
			raw = nil
		}
	}
	return fs, fs.split(raw), nil
}

func (f *fileSource) Name() string { return "tail -f " + f.name }

// Next reads what was appended since the last call. A file that shrank
// was rotated or truncated: it is read again from its start.
func (f *fileSource) Next() ([]entry, error) {
	file, err := os.Open(f.path)
	if err != nil {
		return nil, fmt.Errorf("follow: %w", err)
	}
	defer file.Close()
	st, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("follow: %w", err)
	}
	var out []entry
	if st.Size() < f.off {
		f.off, f.carry = 0, nil
		out = append(out, entry{TS: f.now().Format(tsLayout), Level: lvlInfo, Svc: "logs", Msg: "file truncated, reading from the start"})
	}
	if st.Size() == f.off {
		return out, nil
	}
	n := st.Size() - f.off
	if n > readMax {
		n = readMax
	}
	buf := make([]byte, n)
	if _, err := file.ReadAt(buf, f.off); err != nil && err != io.EOF {
		return out, fmt.Errorf("follow: %w", err)
	}
	f.off += n
	return append(out, f.split(buf)...), nil
}

// split appends raw to the carried partial line and returns an entry for
// every complete line; the rest is carried to the next call.
func (f *fileSource) split(raw []byte) []entry {
	data := append(f.carry, raw...)
	var out []entry
	ts := f.now().Format(tsLayout)
	for {
		i := bytes.IndexByte(data, '\n')
		if i < 0 {
			break
		}
		text := sanitize(string(data[:i]))
		data = data[i+1:]
		if strings.TrimSpace(text) == "" {
			continue
		}
		out = append(out, entry{TS: ts, Level: parseLevel(text), Svc: f.name, Msg: text})
	}
	f.carry = append([]byte(nil), data...)
	return out
}

// parseLevel guesses a line's level from the first word that names one:
// ERROR, WARN, INFO, DEBUG and their usual spellings, in any case. The
// short forms ERR, WRN, INF and DBG count only in capitals, because "err"
// is also a variable name. A line that names no level is info.
func parseLevel(s string) string {
	words := strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) })
	for _, w := range words {
		switch strings.ToUpper(w) {
		case "ERROR", "FATAL", "PANIC", "CRITICAL", "SEVERE":
			return lvlError
		case "WARN", "WARNING":
			return lvlWarn
		case "INFO", "NOTICE":
			return lvlInfo
		case "DEBUG", "TRACE":
			return lvlDebug
		case "ERR":
			if w == "ERR" {
				return lvlError
			}
		case "WRN":
			if w == "WRN" {
				return lvlWarn
			}
		case "INF":
			if w == "INF" {
				return lvlInfo
			}
		case "DBG":
			if w == "DBG" {
				return lvlDebug
			}
		}
	}
	return lvlInfo
}

// sanitize makes a real log line safe to put in the store and on screen:
// ANSI escape sequences are dropped, tabs become a space, and any other
// control character is removed (the runtime would strip it anyway and
// report V007).
func sanitize(s string) string {
	var b strings.Builder
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		switch {
		case r == 0x1b:
			if i+1 < len(rs) && rs[i+1] == '[' { // CSI: skip to the final byte
				i += 2
				for i < len(rs) && (rs[i] < 0x40 || rs[i] > 0x7e) {
					i++
				}
			}
		case r == '\t':
			b.WriteByte(' ')
		case unicode.IsControl(r):
		default:
			b.WriteRune(r)
		}
	}
	return strings.TrimRight(b.String(), " ")
}
