package host

import "github.com/abdul-hamid-achik/tuimark/internal/img"

// ReadRegular is os.ReadFile for a file the caller did not choose: one a
// cloned repository or a workspace can plant under any name, or that a
// data-bound path names. It refuses anything that is not a regular file,
// a symlink to one included, and anything of more than limit bytes
// (MaxReadBytes when limit <= 0), so a FIFO or a link to /dev/zero is an
// error and not a hang or an unbounded read. The implementation is
// img.ReadRegular, which <img> reads its files through as well.
func ReadRegular(path string, limit int64) ([]byte, error) { return img.ReadRegular(path, limit) }

// MaxReadBytes bounds a file ReadRegular reads (64 MiB, the bound `tuimark
// lsp` puts on one protocol message).
const MaxReadBytes = img.MaxReadBytes

// ErrNotRegular is ReadRegular's refusal of a path that is not a regular
// file (a device, a FIFO, a socket, a directory), and ErrTooLarge of one
// with more than the limit.
var (
	ErrNotRegular = img.ErrNotRegular
	ErrTooLarge   = img.ErrTooLarge
)
