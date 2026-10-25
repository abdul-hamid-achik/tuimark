//go:build unix

package img

import (
	"fmt"
	"os"
	"syscall"
)

// openFile is os.OpenFile, a seam for the test that counts opens.
var openFile = os.OpenFile

// openRegular opens path for reading and returns the descriptor for
// ReadRegular to judge by fstat. A Stat of the path comes first, so a
// device node, FIFO or socket that sits there is refused without being
// opened (some devices act on open). That Stat leaves a window: a node
// swapped in between it and the open is opened anyway, so the open is
// made without waiting (O_NONBLOCK: a FIFO does not wait for a writer)
// and without taking a terminal (O_NOCTTY: a terminal device does not
// become the process's controlling terminal), and ReadRegular's fstat of
// the descriptor makes the final call. A path that cannot be opened but
// names something that is not a regular file (a socket swapped in: ENXIO,
// or EOPNOTSUPP on macOS) is ErrNotRegular as well.
func openRegular(path string) (*os.File, error) {
	if fi, err := os.Stat(path); err != nil {
		return nil, err
	} else if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("%s: %w", path, ErrNotRegular)
	}
	f, err := openFile(path, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOCTTY, 0)
	if err != nil {
		if fi, serr := os.Stat(path); serr == nil && !fi.Mode().IsRegular() {
			return nil, fmt.Errorf("%s: %w", path, ErrNotRegular)
		}
		return nil, err
	}
	return f, nil
}
