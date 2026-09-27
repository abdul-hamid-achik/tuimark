//go:build !windows

package host

import (
	"os"
	"os/signal"
	"syscall"
)

// resizeSignal delivers SIGWINCH notifications until stop is closed.
func resizeSignal(stop <-chan struct{}) <-chan struct{} {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGWINCH)
	out := make(chan struct{}, 1)
	go func() {
		defer signal.Stop(sig)
		for {
			select {
			case <-stop:
				return
			case <-sig:
				select {
				case out <- struct{}{}:
				default:
				}
			}
		}
	}()
	return out
}
