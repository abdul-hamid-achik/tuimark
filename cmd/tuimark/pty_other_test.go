//go:build unix && !darwin && !linux

package main

import (
	"errors"
	"os"
)

const ioctlGetTermios = 0

// openPTY has no implementation here; the pty tests skip.
func openPTY() (*os.File, string, error) {
	return nil, "", errors.New("no pty helper on this platform")
}

// setWinsize has no implementation here either.
func setWinsize(f *os.File, cols, rows int) error {
	return errors.New("no pty helper on this platform")
}
