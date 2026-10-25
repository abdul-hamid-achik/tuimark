//go:build unix

package img

import (
	"fmt"
	"os"
	"syscall"
)

// openRegular opens path for reading and returns the descriptor for
// ReadRegular to judge by fstat. It does not look at the path first: a
// Stat before the open leaves a window in which a device node or FIFO
// swapped in is opened anyway, and some devices act on open. O_NONBLOCK
// keeps a FIFO from waiting for a writer, and O_NOCTTY keeps a terminal
// device from becoming the process's controlling terminal. A path that
// cannot be opened but names something that is not a regular file (a
// socket, say) is ErrNotRegular, as it was before.
func openRegular(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOCTTY, 0)
	if err != nil {
		// Some non-regular files cannot be opened for reading at all (a
		// socket: ENXIO, or EOPNOTSUPP on macOS). Say what the path is
		// when it can be told; no open happens here.
		if fi, serr := os.Stat(path); serr == nil && !fi.Mode().IsRegular() {
			return nil, fmt.Errorf("%s: %w", path, ErrNotRegular)
		}
		return nil, err
	}
	return f, nil
}
