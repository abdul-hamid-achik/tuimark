---
title: Dumps and tools
description: The Tuimark dump format and the CLI tools around it - dump, --cells, --styles, play, inspect, validate, preview, fmt, ir - plus the TUIMARK_* environment variables and the session log.
---

# Dumps and tools

Everything Tuimark renders can be printed as data. The CLI commands on this page all build the same frame the terminal would show, deterministically, and never read the environment, so their output is the same on every machine and in CI. That is what makes them useful to an agent, to a test, and to you.

## dump

`tuimark dump FILE` renders one frame at `--cols` × `--rows` (80 × 24 by default), with `--data FILE.json` as the store. The text form has three sections: the grid, the nodes, the diagnostics.

<<< @/snippets/tiny/app.tui{tui}

<<< @/snippets/out/tiny-text.txt{term}

Each node line is `id tag WxH @(x,y)`, with `-` for a node without an id, and ends with `focus`, `selected`, or `checked` when those apply. The `=== errors ===` section is `ok` or one diagnostic per line.

### The JSON dump

`--format json` is the same frame for a program:

<<< @/snippets/out/tiny-json.jsonc{jsonc}

| Field | Holds |
|---|---|
| `cols`, `rows` | the size rendered |
| `ok` | `false` when any diagnostic is an error |
| `focus` | the focused node's id, or `null` |
| `errors` | every diagnostic, warnings included, each with `severity`, `code`, and `msg`, plus `file`, `line`, `col`, `path`, and `id` when known |
| `nodes` | every laid-out node in document order, modals last |
| `grid` | the painted frame: `rows` strings, each exactly `cols` columns wide, no ANSI |
| `wide` | present and `true` when a row holds a wide or multi-code-point character; see `--cells` |

A node has `tag`, its outer rect `x`, `y`, `w`, `h`, and, when they apply: `id`, `text` (a text's content, an input's value), `key` (a row's key), `focused`, `selected`, `checked`, `scroll` (a viewport's offset and extent), and `classes` (version 2 documents). Nodes scrolled out of a viewport are still listed, with coordinates outside it.

The format is versioned and only grows: a new field is optional, and no field changes type or meaning. JSON schemas for it ship in the repository's `schema/` directory.

Exit status: `0` when the frame is `ok`, `2` when it has an error, `1` for a usage or I/O problem.

### --cells

When text has wide characters, a row of `grid` no longer has one character per column. `--cells` adds a per-column map: one entry per cell, row by row, with its cluster (`""` for the second column of a wide one), its width, and the `id` of the box that painted it.

<<< @/snippets/tiny/wide.tui{tui}

<<< @/snippets/out/tiny-wide.jsonc{jsonc}

Index `cells` by column whenever `wide` is `true`. The same map decides which node a mouse click lands on.

### --styles

The grid shows characters, not colors. `--styles` (JSON only) adds the effective `theme` and, per row, the runs of cells that share a foreground, background, and attributes:

<<< @/snippets/out/tiny-styles.jsonc{jsonc}

Colors are exact: `#rrggbb`, an ANSI name, or `default`. `a` lists `bold`, `dim`, `italic`, `underline`, and `reverse` when set. This is how a test checks that `:focus` or a theme token actually changed a cell, by diffing JSON instead of reading ANSI by eye. Add `--theme light` to check the other palette.

## play

`tuimark play FILE` replays input against a document with no terminal: the same key decoding, key handling, focus, and event rules as a running app. It prints the final frame plus every action that fired.

<<< @/snippets/out/hello-play-json.jsonc{jsonc}

Steps come from `--input "STEP STEP …"` (separated by spaces) or `--script FILE.ndjson` (one JSON object per line):

| `--input` step | `--script` line | Does |
|---|---|---|
| `enter`, `q`, `ctrl+a`, `shift+tab`, … | `{"key": "enter"}` | one key: a [key token](/reference/keys) |
| `text:deploy` | `{"text": "deploy"}` | typed text; a run of characters into an input is one edit |
| `paste:STR` | `{"paste": "…"}` | one bracketed paste; it never reaches the keymap |
| `set:PATH=JSON` | `{"set": {"path": "…", "value": …}}` | `Set(PATH, value)` from the host |
| `focus:#id` | `{"focus": "#id"}` | `Set("@focus", "#id")` |
| `set:@theme="light"` | `{"theme": "light"}` | the host's theme |
| `resize:100x30` | `{"resize": [100, 30]}` | the terminal is resized |
| `click:X,Y` | `{"click": [X, Y]}` | a left click at a cell (version 2, mouse on) |
| `wheel-up:X,Y`, `wheel-down:X,Y` | `{"wheel": "up", "at": [X, Y]}` | one wheel step at a cell |

- Host actions are recorded, never run. A `set:` step stands in for what a handler would do, so a whole interaction can be scripted: `--input "enter set:confirm_open=true tab enter"`.
- `quit` ends the session; later steps are skipped.
- `--format json` adds `"events"`, each `{step, action, source, keys, value}`; `--frames` adds one settled frame per step. `--styles` and `--cells` work as for `dump`.
- A step that cannot be applied (an unknown key, a click outside the grid, a bad path) exits 1 and names the step on stderr.

## inspect

`tuimark inspect` answers "why does this look like that?" for one node. Pick it by id, or by a cell with `--at X,Y`, the node a click there would start from:

<<< @/snippets/out/hello-inspect.txt{term}

It prints the node, its layout path, and its pseudo-classes, then one line per property that is not at its initial value: the computed value, where it came from (`ua` for the built-in sheet, `attribute`, `author`, `inline`, or `inherited`), the winning rule with its file, line, selector, and specificity, and the rules it overrode. `--json` prints the same as JSON, plus every class the node could have, with the guard behind it and whether any rule uses it: the quickest way to find a class that no selector matches.

## validate

`tuimark validate FILE` reports every diagnostic without printing a frame. It checks the whole document, including other screens, closed modals, and false `if` branches, and lays it out at 40, 80, and 120 columns to find layout problems at the sizes that matter (under both themes, for `theme="auto"`).

```sh
tuimark validate app.tui                          # parse and style checks
tuimark validate app.tui --data sample.json       # plus every path against the data
tuimark validate app.tui --strict                 # missing paths become errors
tuimark validate app.tui --catalog actions.json   # unknown action names become B004
tuimark validate app.tui --json                   # diagnostics and the action catalog as JSON
```

Bind checks need data: without `--data`, every path would read as missing, so they are skipped. `--catalog` takes a JSON array of action names, or `{"actions": [...]}`.

## preview

`tuimark preview FILE` prints the text dump and, when stdout is a terminal, paints the frame with ANSI colors first. `--watch` repaints whenever the document, its stylesheets, or the data file change. `--color truecolor|256|16|none` picks the color depth; otherwise it follows `TUIMARK_COLOR`, then the terminal's own capabilities.

## fmt

`tuimark fmt FILE` prints the document in canonical form; `--write` rewrites it and `--check` exits 2 when it is not formatted. See [formatting](/guide/language#formatting).

## ir

`tuimark ir FILE` prints the parsed document as JSON: every node with its attributes, bindings, actions, and its own inline style, before any stylesheet is applied, plus the keymap and the resolved theme tokens. It is the document's structure as the runtime understands it. A schema for it ships in `schema/`.

## Environment variables

Only a running app (`Run()`) and `preview` read the environment. The tools above never do.

| Variable | Values | Effect |
|---|---|---|
| `TUIMARK_THEME` | `dark`, `light` | overrides the theme, like `--theme` |
| `TUIMARK_COLOR` | `truecolor`, `256`, `16`, `none` | forces the color depth instead of detecting it from `TERM`, `COLORTERM`, `NO_COLOR`, and friends |
| `TUIMARK_SYNC` | `0`, `1` | turns synchronized output off or on instead of asking the terminal |
| `TUIMARK_LOG` | a file path | writes an NDJSON log of the session |
| `COLORFGBG` | set by some terminals | the fallback for `theme="auto"` when the terminal does not answer the background query |

Values are exact; an empty value counts as unset. An invalid value makes `Run` return an error before it touches the terminal.

### TUIMARK_LOG

A running app owns the terminal, so it cannot print diagnostics there. `TUIMARK_LOG=session.ndjson ./myapp` writes one JSON object per line instead: the start, the terminal's capabilities and the resolved theme, then every key, paste, mouse event, action, frame (with how long it took and how many bytes it wrote), and resize, and finally why it ended.

```sh
TUIMARK_LOG=session.ndjson ./myapp
jq -c 'select(.ev == "action")' session.ndjson
```

The file is created with mode `0600`. While a `secret` input has focus, keys and pastes are logged as redacted, and that input's events never carry its value.
