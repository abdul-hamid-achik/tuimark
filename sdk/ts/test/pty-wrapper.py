#!/usr/bin/env python3
"""Runs a real `tuimark host` on a pseudo-terminal, for real-child.test.ts.

usage: pty-wrapper.py BINARY ARGS...

`tuimark host` needs a terminal on fds 0 to 2, which `bun test` does not
have. This wrapper gives the child one (answering its capability probe like a
terminal that supports nothing special) and passes fd 3 and fd 4 through, so
spawnHost can use it as `bin`. It forwards SIGTERM and SIGHUP and exits with
the child's code (128 + the signal number when a signal ended it). Test
helper only: it is not part of the package.
"""
import fcntl
import os
import pty
import select
import signal
import struct
import sys
import termios

PROBE_REPLIES = b"\x1b[?2026;2$y\x1b[?2027;2$y\x1b[?64;1;22c"


def main() -> int:
    binary, args = sys.argv[1], sys.argv[2:]
    master, slave = pty.openpty()
    fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 24, 80, 0, 0))
    pid = os.fork()
    if pid == 0:
        os.setsid()
        fcntl.ioctl(slave, termios.TIOCSCTTY, 0)
        for fd in (0, 1, 2):
            os.dup2(slave, fd)
        os.close(master)
        os.close(slave)
        os.execv(binary, [binary] + args)
    os.close(slave)

    for sig in (signal.SIGTERM, signal.SIGHUP):
        signal.signal(sig, lambda s, _frame: os.kill(pid, s))

    answered = False
    tail = b""
    status = None
    while status is None:
        try:
            ready, _, _ = select.select([master], [], [], 0.05)
            if ready:
                data = os.read(master, 65536)
                if not data:
                    raise OSError
                tail = (tail + data)[-64:]
                if not answered and b"\x1b[c" in tail:
                    answered = True
                    os.write(master, PROBE_REPLIES)
        except OSError:
            # The child closed the terminal; collect it.
            _, status = os.waitpid(pid, 0)
            break
        except InterruptedError:
            pass
        done, st = os.waitpid(pid, os.WNOHANG)
        if done:
            status = st
    if os.WIFEXITED(status):
        return os.WEXITSTATUS(status)
    return 128 + os.WTERMSIG(status)


sys.exit(main())
