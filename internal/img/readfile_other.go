//go:build !unix

package img

import (
	"fmt"
	"os"
)

// openRegular is os.Open behind a Stat of the path. There is no
// O_NONBLOCK here (js/wasm, Windows), so this Stat is the ONLY guard that
// keeps a FIFO or device out of the open, and it leaves the window the
// unix opener closes: a node swapped in between the Stat and the open is
// opened. ReadRegular's fstat afterwards still refuses to read it.
func openRegular(path string) (*os.File, error) {
	if fi, err := os.Stat(path); err != nil {
		return nil, err
	} else if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("%s: %w", path, ErrNotRegular)
	}
	return os.Open(path)
}
