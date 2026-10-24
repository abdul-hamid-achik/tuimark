//go:build !unix

package img

import "os"

// openNoBlock is os.Open where there is no O_NONBLOCK; ReadRegular's check
// before the open is what keeps a FIFO out.
func openNoBlock(path string) (*os.File, error) { return os.Open(path) }
