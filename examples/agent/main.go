// Command agent is a small local coding-agent harness built entirely on
// top of tuimark's public API (Load, Bind, Set, On, Dump, Run): the view
// lives in agent.tui + agent.tcss, and this program only supplies data and
// the named actions the view references.
//
//	go build -o bin/agent ./examples/agent
//	./bin/agent                                   # scripted provider, interactive
//	./bin/agent --demo-workspace ./demo-workspace  # seed a workspace first
//	./bin/agent --fixture --provider ollama        # real HTTP client, loopback fake server
//	./bin/agent --headless -p "list" --json        # one shot, machine-readable
//	./bin/agent --dump 120x30 -p "list"             # one shot, then print the UI dump
//
// See README.md for the full walkthrough, including a real local Ollama
// model.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/abdul-hamid-achik/tuimark"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	fs := flag.NewFlagSet("agent", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	dir := fs.String("dir", "examples/agent", "directory with agent.tui and agent.tcss")
	provider := fs.String("provider", "scripted", "scripted or ollama")
	model := fs.String("model", "", "model name (ollama only; ignored by scripted)")
	endpoint := fs.String("endpoint", "http://127.0.0.1:11434", "ollama server base URL")
	fixture := fs.Bool("fixture", false, "start an in-process loopback fake ollama server and point --provider ollama at it")
	mode := fs.String("mode", "build", "plan or build")
	workspace := fs.String("workspace", "workspace", "directory tools are confined to")
	demoWorkspace := fs.String("demo-workspace", "", "recreate this directory with a few sample files and use it as --workspace")
	headless := fs.Bool("headless", false, "no TTY: process -p (if any) and exit")
	var prompt string
	fs.StringVar(&prompt, "p", "", "prompt to send (headless or --dump)")
	fs.StringVar(&prompt, "prompt", "", "alias of -p")
	asJSON := fs.Bool("json", false, "headless: print events as JSON lines")
	approveEdits := fs.Bool("approve-edits", false, "headless: auto-approve edit_file")
	approveCommands := fs.Bool("approve-commands", false, "headless: auto-approve run_command")
	dumpSize := fs.String("dump", "", "process -p (if any), then print the UI dump at COLSxROWS and exit")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *mode != "plan" && *mode != "build" {
		fmt.Fprintln(os.Stderr, "agent: --mode must be plan or build")
		return 2
	}
	if *provider != "scripted" && *provider != "ollama" {
		fmt.Fprintln(os.Stderr, "agent: --provider must be scripted or ollama")
		return 2
	}

	ws := *workspace
	if *demoWorkspace != "" {
		if err := SeedDemoWorkspace(*demoWorkspace); err != nil {
			fmt.Fprintln(os.Stderr, "agent:", err)
			return 1
		}
		ws = *demoWorkspace
	}
	tools, err := NewTools(ws)
	if err != nil {
		fmt.Fprintln(os.Stderr, "agent:", err)
		return 1
	}

	var fx *FixtureServer
	if *fixture {
		fx, err = NewFixtureServer(*model)
		if err != nil {
			fmt.Fprintln(os.Stderr, "agent:", err)
			return 1
		}
		defer fx.Close()
		*endpoint = fx.Endpoint()
		if *model == "" {
			*model = fx.Model
		}
		*provider = "ollama"
	}

	var p Provider
	provName := *provider
	switch *provider {
	case "ollama":
		if *model == "" {
			*model = "qwen3.5:0.8b"
		}
		p = NewOllama(*endpoint, *model, nil)
	default:
		provName = "scripted"
		if *model == "" {
			*model = "scripted-1"
		}
		p = NewScripted()
	}

	ui, err := tuimark.Load(filepath.Join(*dir, "agent.tui"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "agent:", err)
		return 1
	}

	ag := NewAgent(ui, p, provName, *model, tools, ws, *mode)
	ag.Wire()
	if err := ag.Init(); err != nil {
		fmt.Fprintln(os.Stderr, "agent:", err)
		return 1
	}

	if *headless || *dumpSize != "" {
		ag.Headless = true
		ag.ApproveEdits = *approveEdits
		ag.ApproveCommands = *approveCommands
		return runHeadless(ag, prompt, *asJSON, *dumpSize)
	}

	if err := ui.Run(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "agent:", err)
		return 1
	}
	return 0
}
