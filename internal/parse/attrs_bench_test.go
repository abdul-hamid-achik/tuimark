package parse_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/parse"
)

func hugeElement(n int) []byte {
	var b strings.Builder
	b.WriteString("<tui><screen><text")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, ` a%d=""`, i)
	}
	b.WriteString(">x</text></screen></tui>")
	return []byte(b.String())
}

func BenchmarkHugeElement5000(b *testing.B) {
	src := hugeElement(5000)
	for i := 0; i < b.N; i++ {
		parse.Parse(src, "a.tui")
	}
}

func BenchmarkHugeElement20000(b *testing.B) {
	src := hugeElement(20000)
	for i := 0; i < b.N; i++ {
		parse.Parse(src, "a.tui")
	}
}
