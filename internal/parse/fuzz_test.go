package parse

import (
	"os"
	"path/filepath"
	"testing"
)

// FuzzFormat checks that for any input Format accepts (well-formed XML),
// the output is itself accepted and a fixed point: Format(Format(x)) ==
// Format(x). Seeds are the real documents under examples/.
func FuzzFormat(f *testing.F) {
	paths, _ := filepath.Glob("../../examples/*/*.tui")
	more, _ := filepath.Glob("../../examples/*/testdata/*/*.tui")
	for _, p := range append(paths, more...) {
		if b, err := os.ReadFile(p); err == nil {
			f.Add(string(b))
		}
	}
	f.Add("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<tui version=\"1\"><screen/></tui>")
	f.Fuzz(func(t *testing.T, src string) {
		out, err := Format([]byte(src))
		if err != nil {
			return
		}
		again, err := Format([]byte(out))
		if err != nil {
			t.Fatalf("Format output does not re-parse: %v\ninput:  %q\noutput: %q", err, src, out)
		}
		if again != out {
			t.Fatalf("Format is not idempotent\ninput:  %q\nfirst:  %q\nsecond: %q", src, out, again)
		}
		// Parse (the validator) must never panic on what Format accepts.
		Parse([]byte(src), "f.tui")
	})
}
