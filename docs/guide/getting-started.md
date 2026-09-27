---
title: Getting started
description: Install the tuimark CLI, write a first document, and check it with validate, dump, preview and play.
---

# Getting started

Install the CLI, write a three-file document, and look at it the way an agent does: as a validated, dumped frame. Then run it for real from a few lines of Go.

## Install

<InstallPanel heading-level="h3" />

Check that it is on your `PATH`:

```sh
tuimark version
```

## Your first document

A Tuimark view is text. Make a directory with three files: the structure (`app.tui`), the style (`theme.tcss`), and some data to render it with (`sample.json`).

::: code-group

<<< @/snippets/hello/app.tui{tui} [app.tui]

<<< @/snippets/hello/theme.tcss{tcss} [theme.tcss]

<<< @/snippets/hello/sample.json{json} [sample.json]

:::

Reading it top to bottom:

- `<tui version="2">` is the root. Always declare the version; `version="2"` opts into the full vocabulary. See [versions](/guide/language#versions).
- `<style src>` loads the stylesheet, relative to the document.
- The `<keymap>` binds `q` to the built-in `quit`, and `enter` to an action named `open`, but only while the list has focus. The `label`s make them key hints.
- The `<screen>` holds a title, a list, and a `<hints>` bar. `{project}` and `{t.title}` read the JSON data; `each="tasks as t"` repeats the `<item>` once per task; `bind="selected"` ties the list's cursor to a value in the data.
- `on:select="moved"` names an action. Markup never runs code: it only names what happened, and a host program decides what that means.

## Check it

`validate` reads the document and its stylesheet, lays it out at 40, 80, and 120 columns, and reports every problem it finds. `--data` lets it check the bindings too.

<<< @/snippets/out/hello-validate.txt{term}

When something is wrong, each problem is one line: severity, a stable code, the position, and a message. This document has a typo in a tag, a unit that does not exist in a terminal, and an input with no `id`:

<<< @/snippets/hello/broken.tui{tui}

<<< @/snippets/out/hello-broken.txt{term}

The codes are listed in the [diagnostics reference](/reference/diagnostics). The exit status is 2, so a script or an agent can tell right away.

## Look at it

`dump` renders one frame at a size you choose and prints it: the grid of cells, then every laid-out node with its size and position, then the diagnostics.

<<< @/snippets/out/hello-dump.txt{term}

This is the whole point of Tuimark: the frame is data. `--format json` prints the same thing as JSON for a program to read, and the [dump format](/guide/tools#dump) is stable and versioned.

The text grid has no colors. To see them, use `preview`, which paints the frame with ANSI colors when its output is a terminal, and `--watch` repaints it whenever you save:

```sh
tuimark preview app.tui --data sample.json --cols 80 --rows 12 --watch
```

## Drive it

`play` replays input against the document with no terminal at all, and prints the frame it ends on plus every action that fired. Here the list moves down twice, and `enter` fires `open`:

<<< @/snippets/out/hello-play.txt{term}

Each event line reads `STEP ACTION SOURCE KEYS VALUE`: at step 3, `open` fired from the list `tasks`, and `keys` names the row the cursor was on (`t` is the `each` alias). The `selected` marker moved to the third item, and `moved` fired on each step that moved the cursor.

`play` never runs host code: host actions are only recorded. That is what makes it safe to run anywhere, and exactly what you need to check a keymap before any Go exists.

## Run it

A host program loads the document, gives it data, and handles the actions it names. Nothing else: layout, focus, keys, colors, and the terminal stay in the runtime.

```go
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/abdul-hamid-achik/tuimark"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	ui, err := tuimark.Load("app.tui")
	if err != nil {
		return err
	}
	raw, err := os.ReadFile("sample.json")
	if err != nil {
		return err
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return err
	}
	if err := ui.Bind("", data); err != nil { // the whole store
		return err
	}
	ui.On("open", func(ev tuimark.Event) error {
		return ui.Set("project", fmt.Sprintf("opened %v", ev.Keys["t"]))
	})
	return ui.Run(os.Stdout)
}
```

```sh
go mod init hello
go get github.com/abdul-hamid-achik/tuimark@latest
go run .
```

`q` quits: `quit` is a built-in action, so it needs no handler. `moved` has no handler either, so it does nothing. See the [Go API](/guide/go-api) for everything else a host can do.

## Where next

- [Working with agents](/guide/agents): the loop above, as an agent runs it.
- [The .tui language](/guide/language), [TCSS](/guide/tcss), and [Layout](/guide/layout): what you can write.
- [Widgets](/guide/widgets): lists, tables, tabs, inputs, modals, and the rest.
- [Examples](/examples): complete apps from the repository.
