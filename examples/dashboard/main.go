// Command dashboard runs examples/dashboard/app.tui as an interactive
// terminal app: a deploy pipeline chrome showcase.
//
//	go run ./examples/dashboard                 # from the repo root
//	go run ./examples/dashboard -dump 80x24     # print the JSON dump instead
//
// The view lives entirely in app.tui + theme.tcss; this file only loads
// data and implements the named actions: ask_deploy, start_deploy,
// cancel_confirm, goto_logs, goto_main, quit.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/abdul-hamid-achik/tuimark"
)

func main() {
	dir := flag.String("dir", "examples/dashboard", "directory with app.tui, theme.tcss, sample.json")
	dumpSize := flag.String("dump", "", "print the JSON dump at COLSxROWS and exit")
	flag.Parse()
	if err := run(*dir, *dumpSize); err != nil {
		fmt.Fprintln(os.Stderr, "dashboard:", err)
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
	if err := ui.Bind("", data); err != nil {
		return err
	}

	steps := []string{"build", "test", "package", "release"}
	var mu sync.Mutex
	deploying := false

	deploy := func() {
		mu.Lock()
		if deploying {
			mu.Unlock()
			return
		}
		deploying = true
		mu.Unlock()
		defer func() {
			mu.Lock()
			deploying = false
			mu.Unlock()
		}()

		for i, name := range steps {
			_ = ui.Set(fmt.Sprintf("steps.%d.status", i), "running")
			_ = ui.Set("current_step_label", "running: "+name)
			_ = ui.Set("deploy_status", fmt.Sprintf("deploying (%d/%d): %s", i+1, len(steps), name))
			base := i * 100 / len(steps)
			for pct := 0; pct <= 100/len(steps); pct += 5 {
				_ = ui.Set("deploy_progress", base+pct)
				time.Sleep(30 * time.Millisecond)
			}
			_ = ui.Set(fmt.Sprintf("steps.%d.status", i), "done")
		}
		_ = ui.Set("deploy_progress", 100)
		_ = ui.Set("current_step_label", "idle")
		_ = ui.Set("deploy_status", "deployed")
		_ = ui.Set("footer_status", "deployed")
	}

	ui.On("ask_deploy", func(tuimark.Event) error {
		return ui.Set("confirm_open", true)
	})
	ui.On("cancel_confirm", func(tuimark.Event) error {
		return ui.Set("confirm_open", false)
	})
	ui.On("start_deploy", func(tuimark.Event) error {
		if err := ui.Set("confirm_open", false); err != nil {
			return err
		}
		go deploy()
		return nil
	})
	ui.On("goto_logs", func(tuimark.Event) error {
		return ui.Set("@screen", "logs")
	})
	ui.On("goto_main", func(tuimark.Event) error {
		return ui.Set("@screen", "main")
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
