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
