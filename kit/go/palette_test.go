package kit

import (
	"testing"

	"github.com/abdul-hamid-achik/tuimark"
)

func paletteApp(t *testing.T, mode Mode) (*tuimark.App, *Palette, *[]string) {
	t.Helper()
	app := loadComponent(t, "palette", "")
	var ran []string
	run := func(id string) func() error {
		return func() error { ran = append(ran, id); return nil }
	}
	p, err := NewPalette(app, PaletteOptions{Mode: mode},
		Command{ID: "deploy", Title: "Deploy to production", Key: "d", Run: run("deploy")},
		Command{ID: "rollback", Title: "Roll back the last deploy", Key: "r", Run: run("rollback")},
		Command{ID: "logs", Title: "Open the logs", Key: "l", Run: run("logs")},
	)
	if err != nil {
		t.Fatal(err)
	}
	p.Register()
	return app, p, &ran
}

func TestPaletteFiltersAndRuns(t *testing.T) {
	app, _, ran := paletteApp(t, Substring)
	_, text := play(t, app, "ctrl+k")
	if !getBool(t, app, "palette.open") {
		t.Fatal("ctrl+k should open the palette")
	}
	for _, want := range []string{"Deploy to production", "Roll back the last deploy", "Open the logs"} {
		contains(t, text, want)
	}
	_, text = play(t, app, "text:roll")
	contains(t, text, "Roll back the last deploy")
	notContains(t, text, "Open the logs")
	res, text := play(t, app, "enter") // on:submit fires palette_run
	if !equal(*ran, []string{"rollback"}) {
		t.Fatalf("ran %v; events %+v", *ran, res.Events)
	}
	if getBool(t, app, "palette.open") || getStr(t, app, "palette.last") != "Roll back the last deploy" {
		t.Errorf("open %v last %q", getBool(t, app, "palette.open"), getStr(t, app, "palette.last"))
	}
	contains(t, text, "last command: Roll back the last deploy")
}

func TestPaletteArrowsMoveTheCursor(t *testing.T) {
	app, _, ran := paletteApp(t, Substring)
	play(t, app, "ctrl+k", "text:the", "down", "enter") // the, deploy, rollback, logs: "the" matches rollback and logs
	if !equal(*ran, []string{"logs"}) {
		t.Errorf("down then enter ran %v", *ran)
	}
}

func TestPaletteEmptyAndClose(t *testing.T) {
	app, _, ran := paletteApp(t, Substring)
	_, text := play(t, app, "ctrl+k", "text:zzz")
	contains(t, text, "No command matches “zzz”.")
	play(t, app, "enter")
	if len(*ran) != 0 || !getBool(t, app, "palette.open") {
		t.Error("with no match, enter does nothing and the palette stays open")
	}
	play(t, app, "esc")
	if getBool(t, app, "palette.open") {
		t.Error("esc closes the palette")
	}
	// Opening again starts clean.
	play(t, app, "ctrl+k")
	if getStr(t, app, "palette.query") != "" || getStr(t, app, "palette.cur") != "deploy" {
		t.Errorf("query %q cur %q", getStr(t, app, "palette.query"), getStr(t, app, "palette.cur"))
	}
}

func TestPaletteFuzzyAndAdd(t *testing.T) {
	app, p, ran := paletteApp(t, Fuzzy)
	_, text := play(t, app, "ctrl+k", "text:dpl")
	contains(t, text, "Deploy to production")
	notContains(t, text, "Open the logs")
	if err := p.Add(Command{ID: "theme", Title: "Toggle the theme", Key: "t", Run: func() error { *ran = append(*ran, "theme"); return nil }}); err != nil {
		t.Fatal(err)
	}
	play(t, app, "ctrl+u", "text:tgl", "enter")
	if !equal(*ran, []string{"theme"}) {
		t.Errorf("a command added later is found by a fuzzy query: ran %v", *ran)
	}
	// A command with the same ID replaces the old one.
	_ = p.Add(Command{ID: "theme", Title: "Switch the theme"})
	rows := mustGet(t, app, "palette.commands").([]any)
	if len(rows) != 4 || rows[3].(map[string]any)["label"] != "Switch the theme" {
		t.Errorf("commands = %v", rows)
	}
}
