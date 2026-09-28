//go:build unix && !darwin

package host

// isPollerFD is always false here: on Linux an epoll or eventfd
// descriptor has no file type, so validateHostFD's mode check already
// rejects it, and no other unix targeted by this build needs the
// darwin-only kqueue probe (SPEC v0.3b §19.1).
func isPollerFD(fd int) bool { return false }
