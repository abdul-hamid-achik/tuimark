# examples/gitui — a small git client on Tuimark

A lazygit/tig-style git client: Status (staged and unstaged files with a
colored diff preview), Log (a filterable commit table with a commit detail),
and Branches (with a checkout confirmation). The entire view is `app.tui` +
`theme.tcss`; the Go code only keeps the repository's data in the store,
derives a few values, and implements named actions through Tuimark's public
package (`Load`, `Bind`, `Set`, `Get`, `Batch`, `On`, `Derive`, `Every`,
`RunWith`). None of `internal/**` is imported.

By default it shows a fixed fake repository (`sample.json`, "tidepool": a
dozen commits, five branches, nine changed files), so it is deterministic and
safe to try. `-repo PATH` reads a real repository by running `git`.

```
examples/gitui/
  app.tui, theme.tcss   the whole view: layout, style, focus, keymap
  sample.json           the fake repository: raw data, the Derive outputs, and
                        the first preview, so `tuimark dump --data` shows it
  mock.json             fixed store writes for `tuimark run --mock` (no Go)
  main.go               flags, wiring, handlers, Derives, Every, RunWith
  model.go              data shapes + pure parsers (git output, diffs, labels)
  backend.go            the backend interface and the git (os/exec) backend
  sample.go             the in-memory fake repository behind sample.json
  *_test.go             parsers, Derives, Play flows, a temp-repo test
```

## Run it

```
go build -o bin/gitui ./examples/gitui
./bin/gitui                       # the fake repository, from the repo root
./bin/gitui -repo .               # a real repository, READ-ONLY
./bin/gitui -repo . -write        # ... that may also stage, unstage, check out
./bin/gitui -theme light          # dark or light (the document is theme="auto")
./bin/gitui -dump 120x30          # print the frame as JSON and exit
```

`-repo` is **read-only unless `-write` is also given**: without it, `space`,
`a`, `A`, and a checkout answer "read-only (pass -write)" in the status line
and never run a git command that changes anything. In sample mode the stage,
unstage, and checkout actions work on the in-memory fake, which behaves like
git for those (an added file unstages to untracked, a rename unstages to a
deletion plus an untracked file, and so on). `-refresh` (default 3s, `0`
disables) re-reads git in `-repo` mode with `Every`. Run it from the repo
root, or pass `-dir examples/gitui` from elsewhere.

## Keys

| Key | Where | Does |
|---|---|---|
| `1` `2` `3`, `[` `]` | anywhere | Status, Log, Branches; previous/next tab |
| `tab`, `shift+tab` | anywhere | move focus (the tab strip, staged, unstaged, diff) |
| `←` `→` | tab strip | previous/next tab |
| `space` | Status lists | stage / unstage the selected file |
| `a`, `A` | Status lists | stage all, unstage all |
| `j` `k`, arrows, `pgup` `pgdn` | lists, tables, diff | move, scroll |
| `/` | Log | filter by subject or author; `enter` keeps it, `esc` clears it |
| `enter` | Log | open the commit's detail (message, stats, diff) |
| `enter` | Branches | ask to check the branch out; `y` confirms, `n` or `esc` cancels |
| `r` | anywhere | refresh (re-read git in `-repo` mode) |
| `?` | anywhere | every key row, from the document's keymap |
| `q`, `ctrl+c` | anywhere | quit |

Mouse is on: click a tab, a row, or wheel-scroll.

## Preview with no Go

`mock.json` answers the document's actions with fixed store writes, so the
view can be explored without the host (the data is `sample.json`; derived
values such as the staged list or the filtered log are the host's job, so
they do not change here):

```
tuimark run examples/gitui/app.tui --data examples/gitui/sample.json \
            --mock examples/gitui/mock.json
tuimark dump examples/gitui/app.tui --data examples/gitui/sample.json --cols 120 --rows 30
tuimark play examples/gitui/app.tui --data examples/gitui/sample.json --input "2" --cols 100 --rows 26
```

## What it shows

- `<tabs bind>` with `{counts.total}` in a version 3 tab label, `focus=` per
  tab, `switch-to` and `move-next`/`move-prev` built-ins, `<hints/>` and a
  `scope="all"` help modal fed by labeled keymap rows.
- Several `<keymap when>` blocks (`#files`, `#commits:focus`, `#filter`,
  `#diff`, modals) so the hint bar shows only the keys that act now.
- `class:NAME` guards: the status letter's color (M/A/D/R/?), the `+`/`-`/`@@`
  diff lines, the current branch, the `HEAD` ref. Diff lines and commit
  detail are a container `each` with a `scroll` and `focusable`.
- A `<table>` with `<column priority>`: on narrow terminals the log drops
  REFS, then AUTHOR, then WHEN; the branches table drops its extra columns.
- `@media` for a stacked Status tab under 60 columns, `@media (theme: light)`
  for the light palette, `wrap: truncate-start` for paths.
- Modals: commit detail (`focus=` on its scroll), checkout confirmation
  (buttons that take clicks, not focus), help.
- `App.Derive` for everything computed: the staged and unstaged lists, the
  counts ("4 staged · 5 unstaged"), the sync label, and the filtered log and
  its title. The filter input only has a `bind`, so the log filters as you
  type with no handler.
- `App.Every` (auto-refresh in `-repo` mode), `RunWith` + `OnError` (a failing
  git command shows in the status line and the session goes on), `Batch`,
  and `Play`-based tests.

## Tests

```
go test ./examples/gitui/
go test -race ./examples/gitui/
```

Parsers (`git status --porcelain=v1 -b`, `git log`, `git branch -vv`, diffs)
are tested on canned strings; the Derives and every interaction are tested
with `Play` on the sample (stage and unstage, stage all, open a commit,
filter the log, checkout confirm, tabs, help, scrolling); a temp repository
built with `git` in `t.TempDir()` checks the real backend (skipped when git
is not installed).
