//go:build unix

package host

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// OpenHostFDs checks fd 3 and fd 4 as the tuimark host protocol requires
// (SPEC v0.3b §19.1, ValidateHostFDs) and returns them as files: fd 3,
// which the child reads (parent -> child), and fd 4, which it writes
// (child -> parent). fd 4 is set non-blocking first, so os.NewFile hands
// it to the runtime poller: a write to it then parks a goroutine instead
// of a thread, and the final "exit" write can be bounded with a write
// deadline (§19.1 step 4: "sets fd 4 non-blocking for that write and gives
// up after 1 second").
func OpenHostFDs() (fd3, fd4 *os.File, err error) {
	if err := ValidateHostFDs(); err != nil {
		return nil, nil, err
	}
	if err := unix.SetNonblock(4, true); err != nil {
		return nil, nil, fmt.Errorf("fd 4: %w", err)
	}
	return os.NewFile(3, "fd 3"), os.NewFile(4, "fd 4"), nil
}

// ValidateHostFDs checks fd 3 and fd 4 as the tuimark host protocol
// requires (SPEC v0.3b §19.1), before the document loads or the terminal
// is touched: each must be a pipe or a socket, not a poller instance; fd
// 3 must be open for reading and fd 4 for writing. A failed check returns
// a usage error naming the problem; cmd/tuimark's `host` subcommand
// prints it on stderr and exits 1 without writing "ready".
//
// The poller check matters because a parent that passes nothing leaves
// fd 3 and fd 4 to whatever the Go runtime already opened there. On
// darwin its kqueue is at fd 3 when main starts, and fstat reports it as
// a FIFO opened read-write like a real pipe would; isPollerFD's kevent
// probe (darwin only: a kqueue instance) tells them apart. On Linux an
// epoll or eventfd descriptor has no file type, so the mode check alone
// already rejects it (isPollerFD is unused there).
func ValidateHostFDs() error {
	if err := validateHostFD(3, true); err != nil {
		return fmt.Errorf("fd 3: %w", err)
	}
	if err := validateHostFD(4, false); err != nil {
		return fmt.Errorf("fd 4: %w", err)
	}
	return nil
}

func validateHostFD(fd int, wantRead bool) error {
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return fmt.Errorf("not open (%v); the parent must pass fd 3 and fd 4", err)
	}
	switch st.Mode & unix.S_IFMT {
	case unix.S_IFIFO, unix.S_IFSOCK:
	default:
		return fmt.Errorf("must be a pipe or a socket the parent passed")
	}
	if isPollerFD(fd) {
		return fmt.Errorf("is the runtime's own poller, not a pipe or a socket the parent passed")
	}
	flags, err := unix.FcntlInt(uintptr(fd), unix.F_GETFL, 0)
	if err != nil {
		return fmt.Errorf("fcntl: %w", err)
	}
	acc := flags & unix.O_ACCMODE
	if wantRead && acc != unix.O_RDONLY && acc != unix.O_RDWR {
		return fmt.Errorf("must be open for reading")
	}
	if !wantRead && acc != unix.O_WRONLY && acc != unix.O_RDWR {
		return fmt.Errorf("must be open for writing")
	}
	return nil
}
