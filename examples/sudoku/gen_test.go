package main

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// The 81 cells of the board, the number pad, and the notes grid are many
// copies of one element each, with ids by screen position. They are not
// written by hand: this file generates them between the markers
//
//	<!-- generated: NAME -->
//	<!-- end generated -->
//
// of app.tui, and TestAppTuiInSync fails when the file differs from what
// the generator makes. To change them, edit the generator and run
//
//	UPDATE_TUI=1 go test ./examples/sudoku -run TestAppTuiInSync
//
// The output is already in the form `tuimark fmt` writes, so the document
// passes `tuimark fmt --check` as it stands (the gate runs it; an example
// cannot call the formatter from a test).

// flags are the class: guards of a board cell, in the order they are written.
var cellFlags = []string{"cur", "peer", "same", "given", "bad", "wrong", "note", "done"}

func genBoard() []string {
	var out []string
	for br := 0; br < 3; br++ {
		out = append(out, `<row class="band">`)
		for bc := 0; bc < 3; bc++ {
			b := br*3 + bc
			out = append(out, fmt.Sprintf(`  <box id="b%d" class="box" class:cur="boxes.b%d">`, b, b))
			for r := br * 3; r < br*3+3; r++ {
				out = append(out, `    <row class="line">`)
				for c := bc * 3; c < bc*3+3; c++ {
					id := fmt.Sprintf("c%d%d", r, c)
					var guards []string
					for _, f := range cellFlags {
						guards = append(guards, fmt.Sprintf(`class:%s="cells.%s.%s"`, f, id, f))
					}
					out = append(out, fmt.Sprintf(`      <text id="%s" class="cell" %s on:click="cell">{cells.%s.t}</text>`,
						id, strings.Join(guards, " "), id))
				}
				out = append(out, `    </row>`)
			}
			out = append(out, `  </box>`)
		}
		out = append(out, `</row>`)
	}
	return out
}

func genPad() []string {
	var out []string
	for r := 0; r < 3; r++ {
		out = append(out, `<row class="padrow">`)
		for c := 0; c < 3; c++ {
			d := r*3 + c + 1
			out = append(out, fmt.Sprintf(`  <text id="p%d" class="pad" class:done="pad.p%d" on:click="put%d">%d</text>`, d, d, d, d))
		}
		out = append(out, `</row>`)
	}
	return out
}

func genMini() []string {
	var out []string
	for r := 0; r < 3; r++ {
		out = append(out, `<row class="mrow">`)
		for c := 0; c < 3; c++ {
			d := r*3 + c + 1
			out = append(out, fmt.Sprintf(`  <text class="m" class:on="mini.on%d">{mini.n%d}</text>`, d, d))
		}
		out = append(out, `</row>`)
	}
	return out
}

var generators = map[string]func() []string{"board": genBoard, "pad": genPad, "mini": genMini}

// regenerate rewrites every generated region of src.
func regenerate(src string) (string, error) {
	lines := strings.Split(src, "\n")
	var out []string
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		out = append(out, line)
		t := strings.TrimSpace(line)
		if !strings.HasPrefix(t, "<!-- generated: ") {
			continue
		}
		name := strings.TrimSuffix(strings.TrimPrefix(t, "<!-- generated: "), " -->")
		gen, ok := generators[name]
		if !ok {
			return "", fmt.Errorf("no generator %q", name)
		}
		indent := line[:len(line)-len(strings.TrimLeft(line, " "))]
		for _, l := range gen() {
			out = append(out, indent+l)
		}
		for i++; i < len(lines) && strings.TrimSpace(lines[i]) != "<!-- end generated -->"; i++ {
		}
		if i == len(lines) {
			return "", fmt.Errorf("region %q has no end marker", name)
		}
		out = append(out, lines[i])
	}
	return strings.Join(out, "\n"), nil
}

func TestAppTuiInSync(t *testing.T) {
	raw, err := os.ReadFile("app.tui")
	if err != nil {
		t.Fatal(err)
	}
	want, err := regenerate(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	if os.Getenv("UPDATE_TUI") != "" {
		if err := os.WriteFile("app.tui", []byte(want), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	if want != string(raw) {
		t.Error("app.tui differs from its generator; run UPDATE_TUI=1 go test ./examples/sudoku -run TestAppTuiInSync")
	}
}
