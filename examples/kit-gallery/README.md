# kit-gallery

Every component of the [Tuimark kit](../../kit/README.md) in one app, on five
tabs. It is the kit's showroom and its integration test: the document is built
only from the kit's classes, loads `kit/theme.tcss` and `kit/kit.tcss`, and has
a small stylesheet of its own (`app.tcss`) for the sizes a page decides.

```sh
go build -o bin/tuimark ./cmd/tuimark
go run ./examples/kit-gallery                  # from the repo root
go run ./examples/kit-gallery -theme light
go run ./examples/kit-gallery -dump 120x40     # Dump() as JSON, no terminal

# the view alone, with no Go: a mock answers every action with fixed writes
tuimark run examples/kit-gallery/app.tui --data examples/kit-gallery/sample.json --mock examples/kit-gallery/mock.json
```

The mock writes fixed values, so a preview opens and closes the dialogs and
shows toasts, but filtering the palette, validating the form and appending to
the log need the Go host (they compute).

## Tabs

| Tab | Components |
|---|---|
| 1 basics | breadcrumb, badge, button, alert, stat (with sparkline), labeled progress, card |
| 2 forms | form field with validation (`class:invalid`), menu with checked marks |
| 3 data | data table (header, cursor row, right-aligned numbers, `priority` columns), log panel (`stick="bottom"`), empty state |
| 4 overlays | confirm dialog, command palette, help overlay, toasts in four variants |
| 5 layouts | split pane (master / detail); the page itself is app bar, tabs, key hints, toast slot, status bar |

## Keys

`1`-`5` / `[` `]` switch tabs, `ctrl+k` opens the command palette, `?` the help,
`t` toggles the theme, `backspace` goes up a breadcrumb, `esc` dismisses a toast,
`q` quits. Per widget: `enter` opens a service (3), `a` appends a log line (3),
`space` toggles a notification channel (2). The key hints at the bottom show
the ones that apply to the focused widget.

## The host

`main.go` and `handlers.go` have no layout code. They implement the named
actions and keep two derived values:

- `Derive("found", …)`: the palette's commands filtered by `palette.query`.
  The list in the markup reads `found.items`, so typing filters with no handler.
- `Derive("detail", …)`: the selected incident of the split pane.
- `Every(time.Second, tick)`: counts a toast (3 s) and the status line (5 s)
  down and clears them.
- handlers: dialogs open and close by flipping `<instance>.open`; the form
  validates into `forms.profile.name.error` / `forms.profile.email.error` and moves focus to the
  first invalid field on submit; `log_append` adds a line and the panel follows
  it.

The kit's actions are generic (`dialog_confirm`, `toast_show`, `form_validate`);
the instance is the prefix of the event's `source` before the last `__`
(`confirm__yes` is the instance `confirm`; `toast__warn` is the level `warn` of
the toast). `instanceOf` and `partOf` in `main.go` are the two small helpers
that do it; they are meant to be copied into a host, not imported.

`sample.json` holds a literal snapshot of `found` and `detail` so `validate`,
`dump` and `run --mock` work with no Go; a test keeps the snapshot from drifting
(`UPDATE_SAMPLE=1 go test ./examples/kit-gallery -run Snapshot` prints fresh
values).

## Tests

`go test ./examples/kit-gallery/` checks that:

- the gallery and **every** `kit/components/*.tui` have no diagnostic (warnings
  included) at 40, 80 and 120 columns under both themes, with every tab visited
  and every modal and toast open (an inactive tab or a closed modal is not laid
  out, so a clean first screen proves little);
- every class a document writes has a rule, so a typo is not silent, and every
  kit class has a use, so a rename leaves no dead rule;
- the conventions hold in every document and sheet (`kit_test.go`): classes are
  `k-` plus short modifiers used only next to a `k-` class, ids are
  `instance__part`, actions are `component_verb`, tokens used are defined for
  both themes (and the runtime's ten required ones are kept), no sheet outside
  `theme.tcss` holds a literal color, every overlay has `on:escape`, a size and a
  keymap scoped to its own class that comes first, every container writes its
  size, every `tabs` writes `focusable`, no document writes a `version`, and
  every component starts with its contract comment;
- every action has a handler and a `mock.json` entry, and the mock's writes apply;
- keys do what they should with `Play`: tabs switch, the dialog opens and closes,
  the palette filters and runs, the form validates as you type, the log follows,
  the split follows the cursor, a focused button and a focused input change
  their own cells;
- `tick` clears the toast and the status line.
