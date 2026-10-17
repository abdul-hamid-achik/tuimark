//go:build tuimark_verify

package layout

// verifying is set by the tuimark_verify build tag: the engine then also
// computes again what it would otherwise take from its memo across
// passes (scrollLayout's content height), and panics when the two
// differ. `go test -tags tuimark_verify ./...` runs the whole suite that
// way.
const verifying = true
