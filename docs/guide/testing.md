---
title: Testing
description: Testing Tuimark views with golden dumps and tuimark test, Go tests on Dump and Validate, and end-to-end terminal specs with Glyphrun.
---

# Testing

Because every frame is deterministic data, a Tuimark view is easy to test at three levels:

| Level | Tool | Catches |
|---|---|---|
| The document | `tuimark test` with golden dumps | any change to the geometry, text, or styles of a frame, at the sizes you care about |
| The host | `go test` on `Dump`, `Validate`, and `Catalog` | a view that no longer loads, a missing handler, data that renders wrong |
| The whole app | [Glyphrun](https://github.com/abdul-hamid-achik/glyphrun) specs | the real binary in a real terminal: keys, timing, quitting cleanly |

## Golden dumps

A **golden** is a dump saved to a file and committed. `tuimark test` renders each document again and compares, so any change to a frame shows up as a failing test and a readable diff.

The goldens are listed in a manifest, `testdata/golden/manifest.json` by default:

```json
[
  {
    "name": "tasks",
    "file": "ui/app.tui",
    "data": "ui/sample.json",
    "sizes": ["40x24", "80x24", "120x30"],
    "json": ["80x24"]
  },
  {
    "name": "tasks-after-enter",
    "file": "ui/app.tui",
    "data": "ui/sample.json",
    "input": "down enter",
    "sizes": ["80x24"],
    "json": ["80x24"],
    "styles": true
  }
]
```

For every size in `sizes`, the text dump is compared with `<dir>/<name>/<size>.txt`; for the sizes also in `json`, the JSON dump with `<size>.json`.

| Field | Meaning |
|---|---|
| `name`, `file`, `sizes` | required: the golden's directory, the document, and the sizes to render |
| `data` | the JSON store, as `--data` |
| `json` | the sizes that also get a JSON golden |
| `theme` | `"dark"` or `"light"`, as `--theme` |
| `input` or `script` | [`play`](/guide/tools#play) steps: the golden is then the frame after them, with the events that fired |
| `styles`, `cells` | include `--styles` or `--cells` in the JSON goldens |
| `frozen` | `true` means `--update` never rewrites this golden |

```sh
tuimark test                      # compare; exit 2 on any mismatch
tuimark test --update             # rewrite the goldens that are not frozen
tuimark test path/to/goldens      # another directory with its own manifest.json
```

Each golden prints one `PASS` or `FAIL` line, with a short diff hint on failure.

### Updating safely

`--update` protects the JSON goldens: it refuses to overwrite one unless the new dump is a **superset** of the old, with every existing value unchanged and every array the same length, only gaining members. If a node moved or text changed, it writes nothing for that size and fails naming the first difference as a JSON Pointer:

```text
FAIL tasks 80x24 (json): update removes or changes /nodes/3/y
```

When the change is deliberate (you moved the node), review it and run `tuimark test --update --allow-breaking` once. Accidental layout changes can no longer slip in with an update.

## Go tests

In the host's own tests, `Validate`, `Dump`, and `Catalog` check the view with the real data types, without a terminal. `Dump` is a pure snapshot, so it is safe to call at many sizes in one test. The [Go API](/guide/go-api#dump-and-validate) page has a complete example; a check that every action has a handler is a few more lines:

```go
for _, a := range ui.Catalog() {
	if !a.Builtin && !a.Registered {
		t.Errorf("action %q (used by %v) has no handler", a.Name, a.Sources)
	}
}
```

## End-to-end with Glyphrun

[Glyphrun](https://github.com/abdul-hamid-achik/glyphrun) runs a program in a real pseudo-terminal, types into it, waits for text on the screen, and checks outcomes, writing an artifact pack (screens, frames, an SVG of the final screen) that a person or an agent can read after a failure. Tuimark's own repository tests every example this way.

A spec for the host from [getting started](/guide/getting-started#run-it):

```yaml
version: 1
name: hello_open
intent: |
  The checklist opens with the first task selected; moving down and pressing
  enter opens the second task, and q quits cleanly.
target:
  cmd: ["./bin/hello"]
  cwd: "."
  env:
    TUIMARK_THEME: dark
    TUIMARK_COLOR: truecolor
terminal:
  cols: 60
  rows: 12
steps:
  - wait:
      screen:
        contains: "launch checklist"
      timeoutMs: 5000
  - press: down
  - press: enter
  - wait:
      screen:
        contains: "opened t2"
      timeoutMs: 2000
  - snapshot: opened
  - press: q
  - wait:
      process:
        exitCode: 0
      timeoutMs: 3000
outcomes:
  - id: enter_opens_the_cursor_row
    description: enter on the second task fires open, and the host shows it in the title
    verify:
      screen:
        contains: "opened t2"
  - id: q_quits
    description: q is the built-in quit and the process exits 0
    verify:
      process:
        exitCode: 0
```

```sh
go install github.com/abdul-hamid-achik/glyphrun/cmd/glyph@latest
go build -o bin/hello .
glyph spec verify specs/hello_open.yml
glyph run specs/hello_open.yml
```

Pin `TUIMARK_THEME` and `TUIMARK_COLOR` in the spec (or in `glyphrun.config.yml`), so a run does not depend on the terminal of whoever runs it. `glyph docs` covers the spec format.

## Which to write

- For a view you are designing, let the [agent loop](/guide/agents) (`validate`, `dump`, `play`) find problems as you go, then pin the result with goldens at 40, 80, and 120 columns.
- For interactions, add goldens with `input`: they check the keymap, focus, and events, and run in milliseconds.
- For the host, a Go test on `Catalog` and `Dump` with real data.
- For the finished app, one Glyphrun spec per user journey: launching, the main task, quitting.
