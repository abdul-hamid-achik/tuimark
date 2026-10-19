package play

import (
	"strings"
	"testing"
)

// FuzzParseSteps checks the --input step grammar on any string: no panic,
// the result is deterministic, and every step it returns keeps the raw
// token it was parsed from.
func FuzzParseSteps(f *testing.F) {
	for _, s := range []string{
		"", " ", "tab down enter", "text:hello paste:x set:a=1 focus:#q resize:80x24 click:3,4 wheel-up:0,0 wheel-down:1,2",
		"text:", "paste:", "set:", "set:a", "set:a=", "set:a=notjson", `set:a.b={"x":[1,2]}`, "set:@theme=\"light\"",
		"resize:", "resize:0x0", "resize:1001x1", "resize:-1x5", "resize:axb", "resize:80", "click:", "click:1", "click:1,", "click:-1,0",
		"click:99999999999999999999,0", "wheel-up:1,1,1", "focus:", "ctrl+c ctrl+i ctrl+m ctrl+,", "space shift+tab", "é", "\x00", "text:\xff",
		strings.Repeat("a ", 500),
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		steps, err := ParseSteps(s)
		steps2, err2 := ParseSteps(s)
		if (err == nil) != (err2 == nil) || len(steps) != len(steps2) {
			t.Fatalf("ParseSteps(%q) is not deterministic", s)
		}
		if err == nil {
			for i, st := range steps {
				if st.Raw == "" || strings.Contains(st.Raw, " ") {
					t.Fatalf("step %d of %q has raw %q", i, s, st.Raw)
				}
			}
		}
		// ParseStep takes any token, spaces included.
		_, _ = ParseStep(s)
	})
}
