//go:build unix

package host

import (
	"errors"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// stopSignals catches the signals that would otherwise kill the process
// with the terminal still raw. release restores their default action.
func stopSignals() (<-chan os.Signal, func()) {
	sig := make(chan os.Signal, 4)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGINT)
	return sig, func() { signal.Stop(sig) }
}

// reraise ends the process with s's default action (the terminal is
// already restored). If s is ignored (nohup), it exits with 128+s.
func reraise(s os.Signal) {
	signal.Reset(s)
	sig, ok := s.(syscall.Signal)
	if !ok {
		os.Exit(1)
	}
	_ = syscall.Kill(os.Getpid(), sig)
	time.Sleep(250 * time.Millisecond)
	os.Exit(128 + int(sig))
}

// pollWait is how long one wait for input blocks, so the reader notices
// that the loop returned.
const pollWait = 50 * time.Millisecond

// maxSelectFd is the first descriptor an FdSet cannot hold.
const maxSelectFd = int(unsafe.Sizeof(unix.FdSet{})) * 8

// inputReady returns, for a file input, a function that waits up to
// pollWait for it to become readable and reports whether it did (or
// whether it can no longer be waited on, so the next read reports the
// problem). It uses poll(2), and select(2) for files poll cannot wait on:
// darwin's poll reports POLLNVAL for /dev/tty, which select supports. It
// returns nil for other readers and for files neither can wait on, which
// are read with plain blocking reads.
func inputReady(in io.Reader) func() bool {
	f, ok := in.(*os.File)
	if !ok {
		return nil
	}
	rc, err := f.SyscallConn()
	if err != nil {
		return nil
	}
	fd := -1
	if err := rc.Control(func(s uintptr) { fd = int(s) }); err != nil || fd < 0 {
		return nil
	}
	probe := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
	if _, err := unix.Poll(probe, 0); err == nil && probe[0].Revents&unix.POLLNVAL == 0 {
		return func() bool {
			fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
			n, err := unix.Poll(fds, int(pollWait/time.Millisecond))
			if err != nil {
				return !errors.Is(err, unix.EINTR) && !errors.Is(err, unix.EAGAIN)
			}
			// Readable, hung up, or not pollable (POLLNVAL): read and let
			// the read say which.
			return n > 0
		}
	}
	if fd >= maxSelectFd {
		return nil
	}
	selectReady := func(wait time.Duration) (bool, error) {
		var set unix.FdSet
		set.Set(fd)
		tv := unix.NsecToTimeval(int64(wait))
		n, err := unix.Select(fd+1, &set, nil, nil, &tv)
		return n > 0 && set.IsSet(fd), err
	}
	if _, err := selectReady(0); err != nil {
		return nil
	}
	return func() bool {
		ready, err := selectReady(pollWait)
		if err != nil {
			return !errors.Is(err, unix.EINTR) && !errors.Is(err, unix.EAGAIN)
		}
		return ready
	}
}

// openNoCTTY keeps a terminal that TUIMARK_LOG names from becoming the
// controlling terminal when openRunLog opens it (to refuse it).
const openNoCTTY = syscall.O_NOCTTY
