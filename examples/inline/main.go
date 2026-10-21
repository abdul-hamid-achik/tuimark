// Command inline runs examples/inline/app.tui as an inline session: the
// screen is a live region below the shell's prompt, not the alternate
// screen, and the host prints what it wants to keep into the terminal's
// own scrollback above it with Commit.
//
//	go run ./examples/inline            # from the repo root
//
// Type a message and press enter: the host commits "You: …" and a card
// template's copy as the reply, then clears the composer. ctrl+o shows or
// hides a line of tips (the region grows and shrinks with it), and ctrl+q
// quits, committing a last line before the region is erased. Everything
// above the region is yours to scroll, select, and copy with the
// terminal's own means.
//
// The view lives in app.tui + theme.tcss; this file implements the named
// actions: send, toggle_tips, quit. `tuimark dump FILE --inline` and
// `tuimark play FILE --inline` show the region and the commits without a
// terminal.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/abdul-hamid-achik/tuimark"
)

func main() {
	dir := flag.String("dir", "examples/inline", "directory with app.tui, theme.tcss, sample.json")
	flag.Parse()
	if err := run(*dir); err != nil {
		fmt.Fprintln(os.Stderr, "inline:", err)
		os.Exit(1)
	}
}

func run(dir string) error {
	ui, err := tuimark.Load(filepath.Join(dir, "app.tui"))
	if err != nil {
		return err
	}
	raw, err := os.ReadFile(filepath.Join(dir, "sample.json"))
	if err != nil {
		return err
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return err
	}
	if err := ui.Bind("", data); err != nil {
		return err
	}
	sent := 0
	ui.On("send", func(ev tuimark.Event) error {
		text, _ := ev.Value.(string)
		if strings.TrimSpace(text) == "" {
			return nil
		}
		sent++
		// The commits are printed at once, above the region, and the
		// writes after them are drawn as one frame.
		if err := ui.Commit(tuimark.Commit{Lines: []any{"You: " + text}}); err != nil {
			return err
		}
		_ = ui.Set("last.text", "echo: "+text)
		if err := ui.Commit(tuimark.Commit{Template: "card", With: "last as m", Props: map[string]string{"who": "Claude"}}); err != nil {
			return err
		}
		_ = ui.Set("draft", "")
		_ = ui.Set("status", fmt.Sprintf("%d sent", sent))
		return nil
	})
	ui.On("toggle_tips", func(tuimark.Event) error {
		on, _ := ui.Get("tips")
		return ui.Set("tips", on != true)
	})
	ui.On("quit", func(tuimark.Event) error {
		// What is committed before the session ends stays above the
		// cursor, after the region is erased.
		if err := ui.Commit(tuimark.Commit{Lines: []any{fmt.Sprintf("-- %d messages sent --", sent)}}); err != nil {
			return err
		}
		return tuimark.ErrQuit
	})
	return ui.RunWith(context.Background(), os.Stdout, tuimark.RunOptions{Inline: true})
}
