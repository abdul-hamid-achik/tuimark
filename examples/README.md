# Examples

Each directory is a Tuimark app: a `.tui` document, a `.tcss` stylesheet,
sample data, and, for most, a small Go host. The host supplies data and
implements named actions; it has no layout code. Build any of them from the
repository root:

```sh
go build -o bin/tuimark ./cmd/tuimark
go build -o bin/logs ./examples/logs && ./bin/logs
```

## Apps

| Example | What it is | What it shows |
|---|---|---|
| [`logs`](logs) | A live log viewer, like `tail -f`; `-follow FILE` tails a real file | `<scroll stick="bottom">`, `Every` streaming lines in one `Batch`, `Derive` for the filter and the counts, `<sparkline>`, level colors with `class:` guards |
| [`tasks`](tasks) | A kanban board (Todo, Doing, Done); `-file` keeps it in a JSON file | three focusable lists, `checked` multi-select, chained `Derive`s, a form modal with inputs, a confirm modal, `RunWith`/`OnError` |
| [`gitui`](gitui) | A git client: status, log, branches; `-repo PATH` reads a real repository (read-only without `-write`) | `<tabs>`, tables with `<column priority>`, a colored diff in a `<scroll>`, `<keymap when>` per context, modals |
| [`http`](http) | An HTTP API client, like Postman; a fake API runs in process unless `-base URL` | inputs and a form, a request in a goroutine applied with `Batch`, a spinner on `Every`, cancel with `esc`, response tabs with a scroll and a table |
| [`monitor`](monitor) | A system monitor's studio view | nine tabs, a panel grid with gauges and sparklines, a process table with sorting and multi-select, settings that switch the mouse and the theme |
| [`agent`](agent) | A coding-agent chat on a scripted provider or a local Ollama model | a transcript, a composer, approval modals, a headless mode |
| [`dashboard`](dashboard) | A deploy dashboard | two screens, a progress bar, buttons, a confirm modal |
| [`inbox`](inbox) | A ticket inbox, the SPEC's reference fixture (`version="1"`) | a search input, a bound list, a detail pane, `@media` |
| [`host-ts`](host-ts) | A minimal counter driven by a Bun parent through `@tuimark/host` | `tuimark host` and its fd 3/fd 4 protocol, from TypeScript |

`logs`, `tasks`, `gitui`, and `http` also ship a `mock.json`, so you can try
the view with no Go at all (the mock answers each action with fixed writes;
the Go host is the real app):

```sh
tuimark run examples/tasks/app.tui --data examples/tasks/sample.json --mock examples/tasks/mock.json
```

## Fixtures

These feed the tests rather than show an app:

- [`unicode`](unicode): CJK, emoji, and combining marks in lists, wrapped text, and next to borders.
- [`stacked`](stacked): the inbox layout with static text, for the `@media` goldens.
- [`spike`](spike): the first Tuimark document. Its goldens are frozen.

`tuimark validate` reports nothing for any app here with its sample data.
`logs`, `tasks`, `gitui`, `http`, and `monitor` also have their own Go
tests (`go test ./examples/logs/`, …), which replay keys with `Play` and
check the store and the dump; `agent` has Go tests of its own too.
