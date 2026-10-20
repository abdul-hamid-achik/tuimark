# examples/pipeline — a CI pipeline runner on Tuimark

A simulated CI/CD run in the terminal, in the spirit of GitHub Actions or
Buildkite, built to show animation with `App.Every`. Five stages (build, test,
lint, package, deploy) run one after the other; the jobs of a stage run side by
side. Each job has a status icon (a braille spinner while it runs), a progress
bar that fills in eighths of a cell, and an elapsed time. Beside the job list a
log pane streams the selected job's output and follows its end. The header
shows the pipeline, branch, commit, overall status and progress, and a
sparkline of how many jobs run at once.

The whole view is `app.tui` + `theme.tcss`. The Go code simulates the run and
publishes plain data through Tuimark's public package (`Load`, `Set`, `Get`,
`Batch`, `On`, `Every`, `Derive`, `RunWith`, `Dump`, `Play`); it has no layout,
width, color, or truncation code.

The simulation is deterministic: durations and log lines come from a seeded
generator, so every machine sees the same run. By default one test job
(`integration`) fails on the first run, like a flaky test, and the rest pass.
Select it and press `r` to retry it: the stages after it, held back until now,
go on.

## Run it

```
go build -o bin/pipeline ./examples/pipeline
./bin/pipeline                 # the default run: integration fails, r retries it
./bin/pipeline -flaky=false    # a green run
./bin/pipeline -seed 3         # other durations and log lines
./bin/pipeline -speed 8        # simulated seconds per real second (default 4)
./bin/pipeline -theme light    # Set("@theme", "light") before Run
./bin/pipeline -dump 120x30    # print Dump() as JSON and exit
./bin/pipeline -demo 100x24    # the scripted demo, as JSON lines (see below)
```

Flags: `-dir` (default `examples/pipeline`), `-theme dark|light`, `-seed N`,
`-flaky` (default true), `-speed X` (above 0, at most 1000), `-interval D` (time between ticks, default
100ms), `-dump COLSxROWS`, and `-demo COLSxROWS`.

## Keys

| Key | Does |
|---|---|
| `j` `k`, `up` `down`, `home` `end`, `pgup` `pgdn` | move the job cursor; the log follows the selected job |
| `enter` | focus the log (`esc` goes back to the jobs; `j`/`k` still move the cursor) |
| `r` | retry the selected job, if it failed; the skipped stages queue up again (a canceled run says to press `R` instead) |
| `R` | rerun the whole pipeline as the next run |
| `c` | cancel the run, after a confirmation (`y` cancels, `n` or `esc` keeps going) |
| `f` | pause or resume the log: while paused the pane holds still and its title counts the new lines |
| `g` / `G` | jump to the top / end of the log (while the log follows, `G` sticks it to the end again after you scrolled up; a log paused with `f` stays paused until `f`) |
| `?` | all keys |
| `q`, `ctrl+c` | quit |

The log follows the end of the output by itself until you scroll up
(`<scroll stick="bottom">`). `f` is the other way to read it in peace: it
freezes what the pane shows while the job goes on.

## Preview without Go

`mock.json` gives every action literal values, so the view can be tried with no
host program at all:

```
./bin/tuimark run examples/pipeline/app.tui \
    --data examples/pipeline/sample.json --mock examples/pipeline/mock.json
```

`sample.json` is the run 22 simulated seconds in: build and unit passed,
integration has failed, e2e is running, and the later stages wait. Nothing
animates there and the log does not follow the cursor, since the animation is
`Every` and the log is a `Derive`; use the Go build for the real thing. In the
preview, `r` shows the retry, `R` a fresh run, `c` then `y` a canceled one.

## The animated demo

`go run ./examples/pipeline -demo 100x24 [-theme light]` prints one JSON object
per line, `{"ms": N, "dump": D}`, for the docs site to turn into an animated
SVG. Each frame applies its scripted keys through `App.Play`, so the handlers
run exactly as in `Run`, moves a fake clock one step through the same `tick`
function `Every` calls, and dumps the frame with styles and cells. The script
watches the build pass, moves to `integration`, sees it fail, presses `r`, and
watches the retry pass and deploy finish. The demo ignores `-seed`, `-flaky`, and
`-speed`. The output is byte-identical on every run and OS: a fixed seed, a
fake clock, no map iteration in any output.

## What it shows

- **`App.Every` and a pure step function.** A 100ms tick calls
  `tick(batch, time.Now())`, which moves the simulation to that instant and
  writes the jobs, the clock, and the sparkline history in one `*Batch`. The
  clock is an argument and every random number comes from a plan seeded by
  `(seed, job, attempt)`, so tests and `-demo` drive it with a fake clock and a
  retry or rerun replays the same way however many ticks came before.
- **`App.Derive`.** `rows` (spinner frame, icon, stage label, elapsed time),
  `run` (status badge, mean progress, labels) and `log` (the selected job's
  lines, held back while paused) are recomputed inside every store write, so
  moving the cursor changes the log pane with no handler and the markup has no
  expressions.
- **`<scroll stick="bottom">`**: the log follows the end of a growing
  stream, holds still once you scroll up, and sticks again after `G`.
- **`<progress>` with `bar: eighths`**, per job inside a `<list>` row and for the
  overall run, colored by status with `class:` guards; **`<sparkline>`**;
  **`<hints/>`** from labeled keymap rows (the row for `enter` appears when the
  jobs have focus, the one for `esc` when the log does); a `?` modal with
  `<hints scope="all"/>`; a confirmation `<modal>` with `<keymap when="#confirm">`. The modals' own keys are labeled, so the hint bar names them while a modal is open; the `?` list sits in a `<scroll>` so its `[ close ]` button stays visible on a short terminal.
- **`App.RunWith`** with `OnError`: a handler or tick that fails shows its message
  in the status line and the run goes on.
- **Responsive without code**: `@media (max-cols: N)` drops the sparkline, the
  stage names, and the commit message, and stacks the two panes below 80
  columns, so `validate` is clean at 40, 60, 80, 100, and 120 columns in both
  themes. The confirm and `?` modals are clean there too, down to 40x14, with
  their buttons fully visible (`TestInteractiveStatesAreClean` presses `c` while
  jobs are running and `?` at every size).
- **Light and dark palettes** from `:root` tokens and `@media (theme: light)`.
- **Provenance**: `sample.json` and `mock.json` are read back from the running
  app, derived paths included, so `go test` fails if they drift
  (`UPDATE_SAMPLE=1 go test ./examples/pipeline -run TestSampleInSync` rewrites
  them).

## Files

```
examples/pipeline/
  app.tui, theme.tcss   the whole view: layout, style, focus, keymap
  sample.json           the run 22 simulated seconds in, for `tuimark dump`/`validate --data`
  mock.json             literal action effects for `tuimark run --mock`
  main.go               flags, handlers, the Every tick, RunWith
  sim.go                the simulation: stages, jobs, retry, rerun, cancel (clock injected)
  plan.go               each job attempt's duration, failure point, and log lines (seeded)
  derive.go             the Derive functions (pure)
  demo.go               -demo: the scripted frames as JSON lines
  main_test.go          Validate, Dump at several sizes, Play flows
  sim_test.go           stage order, failure, retry, rerun, cancel, determinism
  demo_test.go          the demo: identical bytes, frame count, total time, clean dumps
  sample_test.go        keeps sample.json and mock.json in sync with the app
```

## Tests

```
go test ./examples/pipeline/
go test -race ./examples/pipeline/
```
