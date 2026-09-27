// Headless mode needs no TTY: it runs at most one turn from -p/--prompt,
// reporting each step as an event (human text, or one JSON object per line
// with --json), and can print the resulting UI dump instead of running the
// interactive loop.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
)

func runHeadless(ag *Agent, prompt string, asJSON bool, dumpSize string) int {
	// --dump wants a single, machine-readable UI snapshot on stdout, so it
	// suppresses the per-step event trace even though a turn still runs.
	switch {
	case dumpSize != "":
		ag.OnEvent = nil
	case asJSON:
		ag.OnEvent = func(ev EngineEvent) {
			b, err := json.Marshal(ev)
			if err != nil {
				return
			}
			fmt.Println(string(b))
		}
	default:
		ag.OnEvent = printPlainEvent
	}

	if prompt != "" {
		ag.RunTurn(context.Background(), prompt)
	}

	if dumpSize != "" {
		cols, rows, err := parseDumpSize(dumpSize)
		if err != nil {
			fmt.Fprintln(os.Stderr, "agent:", err)
			return 2
		}
		d, err := ag.ui.Dump(cols, rows)
		if err != nil {
			fmt.Fprintln(os.Stderr, "agent:", err)
			return 1
		}
		b, err := json.MarshalIndent(d, "", "  ")
		if err != nil {
			fmt.Fprintln(os.Stderr, "agent:", err)
			return 1
		}
		fmt.Println(string(b))
		if !d.OK {
			return 2
		}
	}
	return 0
}

func parseDumpSize(s string) (int, int, error) {
	var cols, rows int
	if _, err := fmt.Sscanf(s, "%dx%d", &cols, &rows); err != nil {
		return 0, 0, fmt.Errorf("--dump wants COLSxROWS, got %q", s)
	}
	if cols <= 0 || rows <= 0 {
		return 0, 0, fmt.Errorf("--dump wants positive COLSxROWS, got %q", s)
	}
	return cols, rows, nil
}

func printPlainEvent(ev EngineEvent) {
	switch ev.Type {
	case "user_message":
		fmt.Println("user:", ev.Text)
	case "assistant_message":
		fmt.Println("assistant:", ev.Text)
	case "tool_call":
		fmt.Println("tool_call:", ev.Tool, ev.Args)
	case "approval":
		fmt.Println("approval:", ev.Tool, "approved =", ev.Approved != nil && *ev.Approved)
	case "tool_result":
		fmt.Println("tool_result:", ev.Tool, "->", ev.Result)
	case "cancelled":
		fmt.Println("cancelled")
	case "error":
		fmt.Println("error:", ev.Text)
	case "mode":
		fmt.Println("mode:", ev.Mode)
	}
}
