package main

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// Regressions from the F2 re-review's open items: what a composition
// diagnostic prints about an id written at another site, and what a copy
// costs to build, stay linear in the document (the owner's decisions:
// every composition diagnostic shows such an id through ir.ShownID, and
// every element a copy places is charged its placement suffix as shown).

// longID is a 100 KB id, valid as an element, use, or template id.
var longID = "a" + strings.Repeat("x", 100000)

// probeRun is one command over a probe: its output, how long it took, and
// how much it allocated.
type probeRun struct {
	code       int
	out, errw  string
	took       time.Duration
	allocated  uint64
	inputBytes int
}

// runProbe writes files into a temporary directory and runs the command
// args (FILE and DATA in them name app.tui and d.json there) in-process.
func runProbe(t *testing.T, files map[string]string, args ...string) probeRun {
	t.Helper()
	dir := writeFiles(t, files)
	input := 0
	for _, c := range files {
		input += len(c)
	}
	for i, a := range args {
		switch a {
		case "FILE":
			args[i] = filepath.Join(dir, "app.tui")
		case "DATA":
			args[i] = filepath.Join(dir, "d.json")
		}
	}
	var r probeRun
	r.inputBytes = input
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	done := make(chan struct{})
	start := time.Now()
	go func() {
		r.code, r.out, r.errw = runCLI(args...)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(60 * time.Second):
		t.Fatalf("%v still running after 60s", args[0])
	}
	r.took = time.Since(start)
	runtime.ReadMemStats(&after)
	r.allocated = after.TotalAlloc - before.TotalAlloc
	return r
}

// bounded checks that r finished within a second (times the race
// slowdown) and 512 MiB of allocation.
func (r probeRun) bounded(t *testing.T, what string) {
	t.Helper()
	if r.code != 0 && r.code != 2 {
		t.Fatalf("%s: exit %d %s", what, r.code, r.errw)
	}
	if r.took > slowdown*time.Second {
		t.Errorf("%s took %v", what, r.took)
	}
	if r.allocated > 512<<20 {
		t.Errorf("%s allocated %d MiB", what, r.allocated>>20)
	}
}

// linear checks that r printed at most ratio times its input.
func (r probeRun) linear(t *testing.T, what string, ratio int) {
	t.Helper()
	if n := len(r.out) + len(r.errw); n > ratio*r.inputBytes {
		t.Errorf("%s printed %d bytes for %d of input (over %dx)", what, n, r.inputBytes, ratio)
	}
}

func texts(n int, body string) string {
	return strings.Repeat(body+"\n", n)
}

// The re-review's open items: a use without id is described by its
// template's id in the placement suffix of every element it places, which
// printed 1 GB for sufA2 (11.4 s, 3.8 GB with --data; 12.9 s without, for
// no output); and the V012 of a use without id quoted a 100 KB element id
// twice per use, 2 GB for sufB (4.1 s, 4.8 GB).
func TestV04bPlacementOutputIsLinear(t *testing.T) {
	sufA2 := map[string]string{
		"app.tui": "<tui>\n<template id=\"" + longID + "\">\n" + texts(10000, "<text>{missing}</text>") + "</template>\n<screen><use template=\"" + longID + "\"/></screen>\n</tui>\n",
		"d.json":  "{}",
	}
	shown := longID[:64] + "…"
	r := runProbe(t, sufA2, "validate", "FILE")
	r.bounded(t, "sufA2 validate")
	if r.out != "ok (bindings not checked: no --data)\n" {
		t.Errorf("sufA2 validate: %.200s", r.out)
	}
	r = runProbe(t, sufA2, "validate", "FILE", "--data", "DATA")
	r.bounded(t, "sufA2 validate --data")
	r.linear(t, "sufA2 validate --data", 10)
	if lines := strings.Split(strings.TrimSuffix(r.out, "\n"), "\n"); len(lines) != 10001 || !strings.HasSuffix(lines[0], `, placed by <use template="`+shown+`"> at app.tui:10004:9`) {
		t.Errorf("sufA2: %d lines, first %.300s", len(lines), lines[0])
	}
	runProbe(t, sufA2, "dump", "FILE", "--data", "DATA", "--format", "json").bounded(t, "sufA2 dump")

	sufB := map[string]string{
		"app.tui": "<tui>\n<template id=\"T\"><text id=\"" + longID + "\">x</text></template>\n<screen>\n" + texts(10000, `<use template="T"/>`) + "</screen>\n</tui>\n",
		"d.json":  "{}",
	}
	r = runProbe(t, sufB, "validate", "FILE", "--data", "DATA")
	r.bounded(t, "sufB validate")
	r.linear(t, "sufB validate", 10)
	if want := fmt.Sprintf(`error V012 app.tui:4:1 /screen/use <use template="T"> needs an id: its template holds ids (#%s), which the use scopes as ID__%s`, shown, shown); !strings.HasPrefix(r.out, want+"\n") {
		t.Errorf("sufB: %.400s\nwant %s", r.out, want)
	}
	runProbe(t, sufB, "dump", "FILE", "--data", "DATA", "--format", "json").bounded(t, "sufB dump")
}

// Every composition diagnostic that quotes an id written at another site
// shows it through ir.ShownID: for each, a probe with a long id prints at
// most about ten times its input (without the rule, each printed the id
// once per copy, element, or reference: hundreds of times its input).
func TestV04bCompositionDiagnosticsAreLinearInLongIDs(t *testing.T) {
	mid := texts(60000, "<text>a</text>")
	for _, c := range []struct {
		name, doc, frag string
		code            string
	}{{
		name: "placement suffix of a use with a long id",
		doc:  "<tui>\n<template id=\"T\">\n" + texts(100, "<text>{missing}</text>") + "</template>\n<screen><use id=\"" + longID[:50000] + "\" template=\"T\"/></screen>\n</tui>\n",
		code: "B003",
	}, {
		name: "placement suffix of a use described by its template",
		doc:  "<tui>\n<template id=\"" + longID + "\">\n" + texts(1000, "<text>{missing}</text>") + "</template>\n<screen><use template=\"" + longID + "\"/></screen>\n</tui>\n",
		code: "B003",
	}, {
		name: "placement suffix of a parse diagnostic",
		doc:  "<tui>\n<template id=\"" + longID + "\">\n" + texts(1000, "<input/>") + "</template>\n<screen><use template=\"" + longID + "\"/></screen>\n</tui>\n",
		code: "V012",
	}, {
		name: "V019 cycle",
		doc:  "<tui>\n<template id=\"" + longID + "\">\n" + texts(1000, `<use template="b"/>`) + "</template>\n<template id=\"b\"><use template=\"" + longID + "\"/></template>\n<screen><text>x</text></screen>\n</tui>\n",
		code: "V019",
	}, {
		name: "V012 of a use without id",
		doc:  "<tui>\n<template id=\"T\"><text id=\"" + longID + "\">x</text></template>\n<screen>\n" + texts(1000, `<use template="T"/>`) + "</screen>\n</tui>\n",
		code: "V012",
	}, {
		name: "V021 of a template definition",
		doc:  "<tui>\n<template id=\"Z1\"><include src=\"mid.tui\"/></template>\n<template id=\"" + longID + "\"><include src=\"mid.tui\"/></template>\n<screen><text>x</text></screen>\n</tui>\n",
		frag: "<fragment>\n" + mid + "</fragment>\n",
		code: "V021",
	}, {
		name: "V011 each alias repeating a with alias",
		doc:  "<tui>\n<template id=\"" + longID + "\">\n" + texts(1000, `<col each="a.i as a" key="a"><text>x</text></col>`) + "</template>\n<screen><use template=\"" + longID + "\" with=\"d as a\"/></screen>\n</tui>\n",
		code: "V011",
	}, {
		name: "B005 to an id a template scopes",
		doc:  "<tui>\n<template id=\"T\"><input id=\"a\"/></template>\n<keymap>\n" + texts(1000, `<bind keys="x" action="focus" to="#a"/>`) + "</keymap>\n<screen><use id=\"" + longID[:50000] + "\" template=\"T\"/></screen>\n</tui>\n",
		code: "B005",
	}, {
		name: "B005 to a use",
		doc:  "<tui>\n<template id=\"T\"><input id=\"" + longID + "\"/></template>\n<keymap>\n" + texts(1000, `<bind keys="x" action="focus" to="#u"/>`) + "</keymap>\n<screen><use id=\"u\" template=\"T\"/></screen>\n</tui>\n",
		code: "B005",
	}} {
		t.Run(c.name, func(t *testing.T) {
			files := map[string]string{"app.tui": c.doc, "d.json": "{}"}
			if c.frag != "" {
				files["mid.tui"] = c.frag
			}
			r := runProbe(t, files, "validate", "FILE", "--data", "DATA")
			r.bounded(t, "validate")
			r.linear(t, "validate", 10)
			// (The id may be shown scoped, as u__a…: what is pinned is
			// that it is cut, and never shown whole.)
			if !strings.Contains(r.out, " "+c.code+" ") || !strings.Contains(r.out, "xxxx…") || strings.Contains(r.out, longID[:65]) {
				t.Errorf("want %s quoting the id shortened:\n%.600s", c.code, r.out)
			}
		})
	}
}

// uses is n uses of template T, one per line, with ids u0, u1, … when
// withIDs (each then scopes the template's ids apart).
func uses(n int, attrs string, withIDs bool) string {
	var sb strings.Builder
	for i := range n {
		if withIDs {
			fmt.Fprintf(&sb, "<use id=\"u%d\" template=\"T\"%s/>\n", i, attrs)
		} else {
			fmt.Fprintf(&sb, "<use template=\"T\"%s/>\n", attrs)
		}
	}
	return sb.String()
}

// A diagnostic reported at an element a copy placed is reported once per
// copy (its placement suffix differs), so every id it quotes from outside
// the element goes through ir.ShownID (the F2 final check): for each one
// parse or layout reports at such an element, a probe whose long id
// stands outside the copies prints at most ten times its input, under a
// second. Before, each printed the id whole once per copy: v011big, 380
// KB, printed 502 MB with 2.9 GB.
func TestV04bDiagnosticsAtCopiesAreLinearInLongIDs(t *testing.T) {
	// 300 copies: what the input holds is mostly the long id, so the
	// ratio measures how often it is printed (a use of 30 bytes may get
	// three diagnostics of 300 bytes each, ids aside, as a written element
	// of 0.4.1 does).
	const n = 300
	L := longID
	for _, c := range []struct {
		name, doc, data string
		codes           []string
	}{{
		name:  "V001 and V004 inside an each template",
		doc:   "<tui>\n<template id=\"T\"><input id=\"i\"/></template>\n<screen><col id=\"" + L + "\" each=\"rows as r\">\n" + uses(n, "", true) + "</col></screen>\n</tui>\n",
		codes: []string{"V001", "V004"},
	}, {
		name:  "V002 inside an each template",
		doc:   "<tui>\n<template id=\"T\"><text focusable=\"true\">a</text><scroll id=\"s\" stick=\"bottom\"><text>b</text></scroll></template>\n<screen><col id=\"" + L + "\" each=\"rows as r\">\n" + uses(n, "", false) + "</col></screen>\n</tui>\n",
		codes: []string{"V002"},
	}, {
		name:  "V011 repeating the alias of an enclosing each",
		doc:   "<tui>\n<template id=\"T\"><col each=\"x.v as r\"><text>a</text></col></template>\n<screen><col id=\"" + L + "\" each=\"" + L[:20000] + " as r\">\n" + uses(n, ` with="r as x"`, false) + "</col></screen>\n</tui>\n",
		codes: []string{"V011"},
	}, {
		name:  "V001 and V002 inside a list item",
		doc:   "<tui>\n<template id=\"T\"><button id=\"b\" label=\"x\"/><text on:click=\"go\">a</text></template>\n<screen><list id=\"" + L + "\"><item>\n" + uses(n, "", true) + "</item></list></screen>\n</tui>\n",
		codes: []string{"V001", "V002"},
	}, {
		name:  "B005 of a tab's focus inside a list item",
		doc:   "<tui>\n<template id=\"T\"><tab id=\"a\" label=\"x\" focus=\"#t1\"><text>b</text></tab></template>\n<screen><list id=\"" + L + "\"><item><text id=\"t1\">a</text></item></list><tabs id=\"tb\">\n" + uses(n, "", true) + "</tabs></screen>\n</tui>\n",
		codes: []string{"B005"},
	}, {
		name:  "B005 of a modal's focus inside a list item",
		doc:   "<tui>\n<template id=\"T\"><modal id=\"m\" open=\"false\" focus=\"#t1\"><text>x</text></modal></template>\n<screen><list id=\"" + L + "\"><item><text id=\"t1\">a</text></item></list>\n" + uses(n, "", true) + "</screen>\n</tui>\n",
		codes: []string{"B005"},
	}, {
		name:  "L006 of a viewport clipped by an enclosing box",
		doc:   "<tui>\n<template id=\"T\"><scroll height=\"5\"><text>a</text></scroll></template>\n<screen><col id=\"" + L + "\" height=\"2\">\n" + uses(n, "", false) + "</col></screen>\n</tui>\n",
		codes: []string{"L006"},
	}, {
		name:  "L007 in a grid container",
		doc:   "<tui>\n<template id=\"T\"><text width=\"5\">a</text></template>\n<screen><col id=\"" + L + "\" style=\"layout: grid; grid-columns: 2\">\n" + uses(n, "", false) + "</col></screen>\n</tui>\n",
		codes: []string{"L007"},
	}} {
		t.Run(c.name, func(t *testing.T) {
			data := c.data
			if data == "" {
				data = `{"rows": []}`
			}
			r := runProbe(t, map[string]string{"app.tui": c.doc, "d.json": data}, "validate", "FILE", "--data", "DATA")
			r.bounded(t, "validate")
			r.linear(t, "validate", 10)
			// Each line of these codes quotes the id shortened, never
			// whole (the written element holding the id may quote its
			// own attributes whole, once).
			for _, code := range c.codes {
				n := 0
				for _, line := range strings.Split(r.out, "\n") {
					if !strings.Contains(line, " "+code+" ") {
						continue
					}
					n++
					if !strings.Contains(line, "xxxx…") || strings.Contains(line, longID[:65]) {
						t.Errorf("want the id shortened, never whole:\n%.600s", line)
						break
					}
				}
				if n == 0 {
					t.Errorf("want %s:\n%.600s", code, r.out)
				}
			}
		})
	}
}

// nestedClassUses is a document of depth templates, each a use of the one
// below it with k classes of its own, the innermost holding w texts, used
// once: each text ends up with depth*k classes, of three letters each (the
// class bytes charged then stay under the byte budget).
func nestedClassUses(depth, k, w int) string {
	const a, b = "abcdefghijklmnopqrstuvwxyz", "abcdefghijklmnopqrstuvwxyz0123456789"
	var sb strings.Builder
	sb.WriteString("<tui>\n<template id=\"T0\">" + strings.Repeat("<text>a</text>", w) + "</template>\n")
	c := 0
	for j := 1; j <= depth; j++ {
		cls := make([]string, k)
		for i := range cls {
			cls[i] = string([]byte{a[c%26], b[c/26%36], b[c/936%36]})
			c++
		}
		fmt.Fprintf(&sb, "<template id=\"T%d\"><use template=\"T%d\" class=%q/></template>\n", j, j-1, strings.Join(cls, " "))
	}
	fmt.Fprintf(&sb, "<screen><col><use template=\"T%d\"/></col></screen>\n</tui>\n", depth)
	return sb.String()
}

// A use's class lands on the elements of its copy once per level of
// nested uses; each name once is restored when the expansion ends, so a
// class costs the same at every level (the F2 final check). Merging
// through a set rebuilt at every level cost the square of the depth:
// k1_20000 (250 levels of one class over 20000 texts) took 6.96 s, and
// s1650 (250 levels of ten over 1650) 5.3 s.
func TestV04bNestedUseClassesAreLinear(t *testing.T) {
	for _, c := range []struct {
		name     string
		k, w     int
		allocMiB uint64
	}{
		{"k1_20000", 1, 20000, 1024},
		{"s1650", 10, 1650, 512},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := runProbe(t, map[string]string{"app.tui": nestedClassUses(250, c.k, c.w)}, "validate", "FILE")
			t.Logf("%s: %v, %d MiB allocated", c.name, r.took, r.allocated>>20)
			if r.code != 0 || r.out != "ok (bindings not checked: no --data)\n" && !strings.HasPrefix(r.out, "warning L009") {
				t.Fatalf("exit %d: %.300s", r.code, r.out)
			}
			if r.took > slowdown*time.Second {
				t.Errorf("took %v", r.took)
			}
			if r.allocated > c.allocMiB<<20 {
				t.Errorf("allocated %d MiB", r.allocated>>20)
			}
		})
	}
}

// The ids an element of a copy quotes from inside its own copy go through
// ir.ShownID too: a child's (L009) and a tab's (B010), and a use's
// template= at the use (V012, V021). Without it, each printed a long id
// once per copy up to the byte budget, 16 MiB for a document a hundred
// times smaller.
func TestV04bCopiesQuoteTheirOwnIDsShortened(t *testing.T) {
	L := longID
	for _, c := range []struct {
		name, doc, data string
		code, want      string
	}{{
		name: "L009 naming a cut child",
		doc:  "<tui>\n<template id=\"T\"><col height=\"1\"><text>a</text><text id=\"" + L + "\">b</text></col></template>\n<screen><col>\n" + uses(100, "", true) + "</col></screen>\n</tui>\n",
		code: "L009", want: "cuts #u0__axxx",
	}, {
		name: "B010 naming the first visible tab",
		doc:  "<tui>\n<template id=\"T\"><tabs id=\"tb\" bind=\"view\"><tab id=\"" + L + "\" label=\"x\"><text>a</text></tab></tabs></template>\n<screen><col>\n" + uses(100, "", true) + "</col></screen>\n</tui>\n",
		data: `{"view": "nope"}`,
		code: "B010", want: "the first visible tab, #u0__axxx",
	}, {
		name: "V012 of a use whose template has a long id",
		doc:  "<tui>\n<template id=\"" + L + "\"><text id=\"e\">x</text></template>\n<template id=\"T\"><use template=\"" + L + "\"/></template>\n<screen><col>\n" + uses(100, "", true) + "</col></screen>\n</tui>\n",
		code: "V012", want: `<use template="` + L[:64] + `…"> needs an id`,
	}, {
		name: "V021 of a use whose template has a long id",
		doc:  "<tui>\n<template id=\"" + L + "\"><text>a</text><!--" + strings.Repeat("x", 9<<20) + "--></template>\n<screen><use template=\"" + L + "\"/><use template=\"" + L + "\"/></screen>\n</tui>\n",
		code: "V021", want: `use template="` + L[:64] + `…" exceeds the expansion limit`,
	}} {
		t.Run(c.name, func(t *testing.T) {
			data := c.data
			if data == "" {
				data = "{}"
			}
			r := runProbe(t, map[string]string{"app.tui": c.doc, "d.json": data}, "validate", "FILE", "--data", "DATA")
			r.bounded(t, "validate")
			r.linear(t, "validate", 10)
			if !strings.Contains(r.out, " "+c.code+" ") || !strings.Contains(r.out, c.want) || strings.Contains(r.out, longID[:65]) {
				t.Errorf("want %s with %q, the id never whole:\n%.600s", c.code, c.want, r.out)
			}
		})
	}
}
