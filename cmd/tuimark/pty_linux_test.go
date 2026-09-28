//go:build linux

package main

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

const ioctlGetTermios = unix.TCGETS

// openPTY opens a new pseudo-terminal: the master and the slave's name.
func openPTY() (*os.File, string, error) {
	fd, err := unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, "", err
	}
	if err := unix.IoctlSetPointerInt(fd, unix.TIOCSPTLCK, 0); err != nil {
		unix.Close(fd)
		return nil, "", err
	}
	n, err := unix.IoctlGetUint32(fd, unix.TIOCGPTN)
	if err != nil {
		unix.Close(fd)
		return nil, "", err
	}
	return os.NewFile(uintptr(fd), "/dev/ptmx"), fmt.Sprintf("/dev/pts/%d", n), nil
}

// setWinsize sets the pty's size, which the child reads as its terminal's.
func setWinsize(f *os.File, cols, rows int) error {
	return unix.IoctlSetWinsize(int(f.Fd()), unix.TIOCSWINSZ, &unix.Winsize{Col: uint16(cols), Row: uint16(rows)})
}
