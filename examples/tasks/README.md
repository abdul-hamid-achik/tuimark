# examples/tasks — a kanban board on Tuimark

A terminal task board: three lanes (Todo, Doing, Done) of cards with a tag,
a priority and a due date. You move cards between lanes, reorder them, mark
several at once, add, edit and delete them through modals, and narrow every
lane with a filter. The whole view is `app.tui` + `theme.tcss`; the Go code
keeps one array of cards and implements named actions through Tuimark's
public package. There is no layout, width, padding, or truncation code in it.

## Files

```
examples/tasks/
  app.tui, theme.tcss   the whole view: lanes, cards, modals, keymap, both themes
  sample.json           the default board (and a snapshot of the derived values)
  mock.json             canned answers for `tuimark run --mock` (no Go needed)
  main.go               flags, load/run, the Every tick, the OnError policy
  handlers.go           the Derive functions and every named action
  board.go              the data model: pure functions, -file load/save
  *_test.go             Dump, Play flows, host logic, the -file round trip
```

## Run it

```
go build -o bin/tasks ./examples/tasks
./bin/tasks                           # in memory, from sample.json
./bin/tasks -file board.json          # load board.json (if any), save after every change
./bin/tasks -theme light              # Set("@theme", "light") before Run
./bin/tasks -today now                # judge due dates against the real date
./bin/tasks -dump 120x30              # print Dump() as JSON and exit
```

Flags: `-dir` (default `examples/tasks`, where `app.tui` lives), `-file`,
`-theme dark|light`, `-today YYYY-MM-DD|now`, `-dump COLSxROWS`. The default
view comes from `sample.json` and a fixed date (2026-10-19), so it is the
same on every machine; the real clock and the file system are opt-in.

## Keys

| Key | Does |
|---|---|
| `tab`, `shift+tab`, `h`/`l`, `←`/`→` | move between the lanes (and the filter) |
| `j`/`k`, `↑`/`↓` | move the cursor in a lane |
| `H` / `L` | move the marked cards (of every lane), else the cursor card, one lane left / right; the cursor follows |
| `K` / `J` | move the card up / down inside its lane |
| `space`, `ctrl+a`, `esc` | mark the card, mark every card in this lane, clear the marks in this lane |
| `n` | new card in the focused lane (a modal: title, tag, due, `ctrl+p` priority) |
| `e` or `enter` | edit the card under the cursor in the same modal |
| `d` | delete the marked cards (of every lane), else the cursor card, after a `y`/`n` confirmation |
| `/` | filter by title, tag or priority (`esc` clears, `enter` or `↓` keeps it) |
| `ctrl+s`, `esc` | in the card form: save, cancel |
| `q`, `ctrl+c` | quit |

## Try the view without Go

```
./bin/tuimark run examples/tasks/app.tui --data examples/tasks/sample.json --mock examples/tasks/mock.json
```

`run` answers every non-built-in action itself from `mock.json`: `n`, `e`
and `d` open their modals, the buttons close them. The built-ins still work
(lane focus with `h`/`l`, the cursor, `space` marks), but moving, saving, and
filtering need the Go host, because the lanes are derived there.
`sample.json` therefore carries a literal copy of the derived values (`cols`,
`counts`, `summary`); a test keeps it in step with what `Derive` computes.

## What it shows

- **A kanban out of three `list`s** bound with `each` + `key` to per-lane
  arrays, a cursor `bind` per lane (`cur.todo`), and `checked` + `mark`
  for multi-select, with `title="Todo · {counts.todo}"` bound titles.
- **`App.Derive`**: `cols` (the lanes, filtered), `counts` and `summary` are
  recomputed inside every store write, each reading the one before it. The
  filter input just binds `filter`; typing narrows the lanes with no handler.
- **`class:NAME` guards** for tag colors (bug, feature, chore), priority
  (`▲ ◆ ▽`), due soon and overdue; done cards dimmed by one CSS rule.
- **Built-in actions** for most of the keymap: `focus` for lane navigation,
  `move-next`/`move-prev`, `check-toggle`/`check-all`/`check-none`.
- **Several `<keymap when>` groups** over the focus chain (`#board`,
  `#todo:focus`, `#filter`, `#form`, `#confirm`), with `<hints>` built from
  their labeled rows and changing as the focus does, also inside the modal.
- **Modals**: `focus=` on the form, `on:escape`, `on:submit` on inputs, and a
  confirmation whose buttons take clicks but not focus, so a stray `enter`
  cannot delete.
- **`App.Every`** (the status line clears after a few seconds; `-today now`
  follows the clock), **`RunWith`** with `OnError` (a failed save lands in the
  status line and the board keeps running), **`Batch`** for atomic updates, and
  `Set("@focus", …)` for the cursor-follows-the-card focus.
- **Both themes** (`theme="auto"`, dark palette in `:root`, light under
  `@media (theme: light)`) and `@media (max-cols: …)` for the narrow layout.

## Tests

```
go test ./examples/tasks/
go test -race ./examples/tasks/
```

They cover a clean `Validate()`, `Dump` at 80x24 and 120x30, every action
having a handler, `mock.json` naming only real actions, `Play` flows (move,
mark and move several, reorder, create/edit through the modal, form errors,
filter, delete with confirmation, quit), the `Derive` output per lane, the
pure move/reorder functions, the `-file` round trip and bad files, a failed
save in the status line, and the `Every` tick called directly.
