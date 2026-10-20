# examples/http — an API client on Tuimark

A small Postman/Insomnia in the terminal: a collection of saved requests and
a history of sent ones, an editor (method, URL, headers, body), and a
response pane with the status, timing, size, content type, the
pretty-printed body, and a table of response headers. The entire view is
`app.tui` + `theme.tcss`; the Go code only sends requests, keeps state, and
implements named actions through Tuimark's public package. There is no
layout, width, padding, or truncation code in it.

```
examples/http/
  app.tui, theme.tcss   the whole view: layout, style, focus, keymap
  sample.json           the default view (a finished POST /users); also the
                         data for `tuimark dump` and `tuimark run`
  mock.json             canned action results for the no-Go preview
  main.go               flags, load(), the Every tick, run()
  actions.go            the named actions: select, edit, save, send, cancel
  derive.go             the Derive functions (filter, counts, flags, summary)
  client.go             the HTTP client: send(), pretty-printing, error text
  fakeapi.go            the in-process demo API (net/http/httptest)
  *_test.go             Validate, Dump, Play flows, client, Derive
```

## Run it

```
go build -o bin/http ./examples/http
./bin/http                                 # demo: an in-process fake API, no network
./bin/http -base https://api.example.com   # {{base}} points at a real server
./bin/http -timeout 3s -theme light
./bin/http -dump 120x30                    # print Dump() as JSON and exit
```

Without `-base` the host starts a `net/http/httptest` server on 127.0.0.1
with a small fake API: `GET /users`, `GET /users/1`, `POST /users` (echoes
the user with a 201), `PUT`/`PATCH`/`DELETE /users/1`, `GET /health`,
`GET /slow` (about 1.5 s), `GET /error` (a 500 with a JSON error), and a 404
for everything else. The collection's URLs use a `{{base}}` placeholder that
the host replaces at send time. Any other URL you type is sent for real, with
a timeout (`-timeout`, default 10s); a URL without a scheme gets `http://`.
Bad URLs, timeouts, refused connections, and cancellations show up in the
response pane and the status line instead of ending the program.

## Keys

| Key | Does |
|---|---|
| `ctrl+s` (or `r`) | send the request; `enter` in the URL input sends too |
| `esc` | cancel the request in flight (clear the filter when it has focus) |
| `s` | save the editor back to the collection (a new or history request is added) |
| `m` (or `ctrl+t`) | cycle the method GET, POST, PUT, PATCH, DELETE |
| `n` | start a new request |
| `/` | filter the collection and the history |
| `1` / `2` | collection or history in the sidebar |
| `b` / `h` | response body or headers |
| `enter` | on a list row: edit it (focus the URL) |
| `D` | in the collection: delete the request |
| `d` | in the headers list: drop the header |
| `j` / `k` | move in a list or the body; `g` / `G` top and bottom of the body |
| `tab` / `shift+tab` | cycle focus |
| `?` | every key |
| `q` / `ctrl+c` | quit |

Selecting a row in the collection loads it into the editor; selecting one in
the history loads the request and its response. Type `Key: Value` in the
`+ header` input and press `enter` to add or replace a header. A focused input
takes printable keys first, so typing a `q` or an `s` into the URL never
quits or saves. Saves live in memory only. The mouse is left off so that you
can select and copy response text with your terminal.

Below 72 columns the sidebar is hidden (a `@media` rule), and below 100 it
narrows.

## Preview without Go

The view runs with no host at all. `--mock` answers the main actions with
canned values (send gives a 200, `m` cycles the method, `s` clears the dirty
dot, `?` opens the key list):

```
go build -o bin/tuimark ./cmd/tuimark
./bin/tuimark run examples/http/app.tui --data examples/http/sample.json \
                  --mock examples/http/mock.json
./bin/tuimark dump examples/http/app.tui --data examples/http/sample.json --cols 100 --rows 30
./bin/tuimark play examples/http/app.tui --data examples/http/sample.json --input "h" --theme light
```

Because the CLI has no `Derive`, `sample.json` also holds the derived values
(`visible`, `history_view`, `counts`, `mf`, `rv`, `summary`, `edit_title`);
a test checks that they agree with what the host computes.

## What it shows

- No `version` (the current language), `theme="auto"`, a dark palette in `:root` and a light one in
  `@media (theme: light)`, `@media (max-cols: …)` for the sidebar, and
  `@media (min-rows: …)` for a taller header list.
- `<tabs>` with `{path}` in v3 tab labels (`collection 10`, `history 3`,
  `headers 6`), `<table>` for response headers, `<scroll>` for the body,
  `<list>` with `class:NAME` guards for the per-method colors and the status
  class colors, `<hints/>` fed by labeled keymap rows grouped with
  `<keymap when>`, and a `<modal>` with `<hints scope="all"/>` as help.
- `wrap: truncate-middle` for the request summary, `bar: eighths` and `dim`
  for the in-flight state, `scrollbar: auto`.
- `App.Derive` for the filtered collection and history, the counts, the
  method and status-class flags, the summary line and the editor title.
- `App.Every` for the spinner and the elapsed time while a request is in
  flight, `Batch` to apply a response as one frame from a goroutine, and
  `context` cancellation (`esc`) all the way into `net/http`.
- `App.RunWith` with `RunOptions.OnError`, so a failing handler shows in the
  status line instead of ending the session.
- Tests with `Play` that inject the request runner, so the store can be
  asserted right after the step that sent, and a client tested against the
  in-process API (status classes, JSON, 404 and 500, timeout, cancel,
  connection refused).

```
go test ./examples/http/
go test -race ./examples/http/
```
