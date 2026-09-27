//go:build unix && !darwin && !linux

package host

import (
	"errors"
	"os"
)

const ioctlGetTermios = 0

// openPTY has no implementation here; the pty tests skip.
func openPTY() (*os.File, string, error) {
	return nil, "", errors.New("no pty helper on this platform")
}
