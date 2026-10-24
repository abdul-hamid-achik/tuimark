//go:build unix

package img

import (
	"os"
	"syscall"
)

// openNoBlock opens path for reading without waiting for a writer, so a
// FIFO swapped in after ReadRegular's check cannot block the open.
func openNoBlock(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
}
