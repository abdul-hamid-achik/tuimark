package img

import (
	"errors"
	"fmt"
	"io"
	"os"
)

// ReadRegular lives here, not in internal/host, because img reads files
// too and host imports img: host re-exports these names (host.ReadRegular,
// MaxReadBytes, ErrNotRegular, ErrTooLarge).

// MaxReadBytes bounds a file ReadRegular reads (64 MiB, the bound `tuimark
// lsp` puts on one protocol message).
const MaxReadBytes = 64 << 20

// ErrNotRegular is ReadRegular's refusal of a path that is not a regular
// file (a device, a FIFO, a socket, a directory), and ErrTooLarge of one
// with more than the limit.
var (
	ErrNotRegular = errors.New("not a regular file")
	ErrTooLarge   = errors.New("file too large")
)

// ReadRegular is os.ReadFile for a file the caller did not choose: one a
// cloned repository or a workspace can plant under any name, which an
// editor server reads on its own (SPEC v0.4a §15.12). It refuses anything
// that is not a regular file, a symlink to one included, and anything of
// more than limit bytes (MaxReadBytes when limit <= 0), so a link to
// /dev/stdin or /dev/zero, or a FIFO, is an error and not a hang, a stolen
// protocol stream, or an unbounded read.
//
// The path is checked before it is opened, because opening a FIFO for
// reading blocks before anything can be asked of the descriptor; the open
// is then non-blocking where the platform has that, and the opened
// descriptor is checked again, so a path swapped in between is caught.
func ReadRegular(path string, limit int64) ([]byte, error) {
	if limit <= 0 {
		limit = MaxReadBytes
	}
	if fi, err := os.Stat(path); err != nil {
		return nil, err
	} else if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("%s: %w", path, ErrNotRegular)
	}
	f, err := openNoBlock(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if fi, err := f.Stat(); err != nil {
		return nil, err
	} else if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("%s: %w", path, ErrNotRegular)
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%s: %w (over %d bytes)", path, ErrTooLarge, limit)
	}
	return data, nil
}
