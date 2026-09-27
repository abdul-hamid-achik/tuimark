// Command inbox runs examples/inbox/app.tui as an interactive terminal app.
//
//	go run ./examples/inbox                 # from the repo root
//	go run ./examples/inbox -dump 80x24     # print the JSON dump instead
//
// The view lives entirely in app.tui + theme.tcss; this file only loads
// data and implements the named actions: open, search, quit.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/abdul-hamid-achik/tuimark"
)

type ticket struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Prio  string `json:"prio"`
	Body  string `json:"body,omitempty"`
}

type sample struct {
	Folder  string   `json:"folder"`
	Tickets []ticket `json:"tickets"`
}

func main() {
	dir := flag.String("dir", "examples/inbox", "directory with app.tui, theme.tcss, sample.json")
	dumpSize := flag.String("dump", "", "print the JSON dump at COLSxROWS and exit")
	flag.Parse()
	if err := run(*dir, *dumpSize); err != nil {
		fmt.Fprintln(os.Stderr, "inbox:", err)
		os.Exit(1)
	}
}

func run(dir, dumpSize string) error {
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
	var s sample
	_ = json.Unmarshal(raw, &s)
	all := s.Tickets
	for i := range all {
		if all[i].Body == "" {
			all[i].Body = fmt.Sprintf("%s ticket in %s. %s. Press / to search, tab to move between the search box and the list, q to quit.", all[i].Prio, s.Folder, all[i].Title)
		}
	}
	if err := ui.Bind("", data); err != nil {
		return err
	}

	show := func(id string) {
		for _, t := range all {
			if t.ID == id {
				_ = ui.Set("selected_ticket", t)
				_ = ui.Set("status", "viewing "+t.ID)
				return
			}
		}
		_ = ui.Set("selected_ticket", nil)
	}
	ui.On("open", func(ev tuimark.Event) error {
		if id, ok := ev.Keys["item"].(string); ok {
			show(id)
		}
		return nil
	})
	ui.On("search", func(ev tuimark.Event) error {
		q, _ := ev.Value.(string)
		q = strings.ToLower(strings.TrimSpace(q))
		var hits []ticket
		for _, t := range all {
			if q == "" || strings.Contains(strings.ToLower(t.Title), q) || strings.Contains(strings.ToLower(t.ID), q) {
				hits = append(hits, t)
			}
		}
		if hits == nil {
			hits = []ticket{}
		}
		_ = ui.Set("tickets", hits)
		_ = ui.Set("count", len(hits))
		if len(hits) > 0 {
			_ = ui.Set("selected", hits[0].ID)
			show(hits[0].ID)
		} else {
			_ = ui.Set("selected_ticket", nil)
		}
		if q == "" {
			_ = ui.Set("status", "all tickets")
		} else {
			_ = ui.Set("status", fmt.Sprintf("%d match %q", len(hits), q))
		}
		return nil
	})
	ui.On("quit", func(tuimark.Event) error { return tuimark.ErrQuit })

	if dumpSize != "" {
		var cols, rows int
		if _, err := fmt.Sscanf(dumpSize, "%dx%d", &cols, &rows); err != nil {
			return fmt.Errorf("-dump wants COLSxROWS, got %q", dumpSize)
		}
		d, err := ui.Dump(cols, rows)
		if err != nil {
			return err
		}
		b, _ := json.MarshalIndent(d, "", "  ")
		fmt.Println(string(b))
		return nil
	}
	return ui.Run(os.Stdout)
}
