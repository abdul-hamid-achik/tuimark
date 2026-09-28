//go:build darwin

package host

import "golang.org/x/sys/unix"

// isPollerFD reports whether fd is a working kqueue instance (SPEC v0.3b
// §19.1): a kevent call with no changes and no events succeeds on one and
// fails (ENOTTY/EINVAL) on a pipe or a socket. Only darwin's runtime can
// put its own kqueue at fd 3 (see ValidateHostFDs).
func isPollerFD(fd int) bool {
	_, err := unix.Kevent(fd, nil, nil, &unix.Timespec{})
	return err == nil
}
