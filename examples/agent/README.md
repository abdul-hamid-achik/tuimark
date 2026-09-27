# examples/agent — a local coding-agent harness on Tuimark

A small, real coding-agent chat UI (transcript, tool approvals, plan/build
modes, a headless mode) where the entire view is `.tui` + `.tcss` markup and
the Go code only supplies data and named actions through Tuimark's public
package (`tuimark.Load/Bind/Set/On/Dump/Run`). None of `internal/**` is
imported here.

```
examples/agent/
  agent.tui, agent.tcss   the whole view: layout, style, focus, keymap
  sample.json             a representative mid-conversation state for
                           `tuimark dump` (see below); not loaded by bin/agent
  main.go                 flags, wiring
  engine.go                the Agent: transcript, tool loop, approval flow
  tools.go, demo.go        the confined tool set + demo workspace seeding
  provider.go              the Provider interface + shared tool catalog
  scripted.go, ollama.go   the two providers
  fixture.go                the in-process loopback fake Ollama server
  headless.go              --headless / --dump
```

## Build

```
go build -o bin/tuimark ./cmd/tuimark
go build -o bin/agent   ./examples/agent
```

## Run it

```
# scripted provider (default), interactive, seeds a fresh demo workspace
./bin/agent --demo-workspace ./demo-workspace

# a real local Ollama model
./bin/agent --provider ollama --model qwen3.5:0.8b --demo-workspace ./demo-workspace

# the loopback fixture: exercises the real HTTP streaming client with no model
./bin/agent --fixture --demo-workspace ./demo-workspace

# headless, machine-readable, one shot
./bin/agent --headless --json --demo-workspace ./demo-workspace -p "list"

# headless, then print the UI's own JSON dump instead of running interactively
./bin/agent --headless --dump 120x30 --demo-workspace ./demo-workspace -p "list"
```

Keys: `ctrl+c` quit, `ctrl+p` toggle plan/build, `ctrl+l` clear transcript,
`esc` cancel an in-flight reply (or deny/close an open modal), `tab` cycle
focus, `?` / `ctrl+h` help, `y`/`n` approve/deny a pending tool while the
approval modal is open.

Try prompts the scripted provider recognizes: `list`, `read FILE`,
`search TEXT`, `edit FILE: OLD -> NEW`, `run CMD ARGS`. Anything else gets a
canned explanation. `edit_file` and `run_command` ask for approval in build
mode (`y`/`n`, or the Approve/Deny buttons) and are refused outright in plan
mode (`ctrl+p`); `list_files`, `read_file`, and `search` never need approval.

## The two providers

- **scripted** (default): deterministic and in-process, so it needs no
  network and is what the fast Go tests and most glyph specs use. It parses
  the prompt shapes above into a tool call, then streams its final answer
  token by token with a small delay.
- **ollama**: a real HTTP client against a local Ollama server's streaming
  `/api/chat` (NDJSON, tool definitions included in the request). Point
  `--endpoint`/`--model` at a real server, or pass `--fixture` to start an
  in-process loopback fake server (see `fixture.go`) that answers with a
  scripted `read_file` tool call and then a final message — this is what
  `agent_fixture_ollama.yml` and `TestFixtureServerRoundTrip` exercise, so
  the real streaming client is proven end to end without a model.

## Tools and the workspace sandbox

`list_files`, `read_file` (bounded read), `search` (bounded substring grep),
and `edit_file` (exact, unique `old_string` → `new_string`; fails if
`old_string` is missing or ambiguous) are confined to `--workspace` (default
`workspace`, created if missing). A path that tries to leave the workspace —
`..`, an absolute path, or a symlink that resolves outside — is rejected.

`run_command` (argv only, no shell, timeout, bounded output) is different: it
runs with the workspace as its working directory, but its arguments are
**not** sandboxed the way the four tools above are. As a best-effort guard,
an argv element that is an absolute path or contains a `..` segment is
rejected, but that only catches the obvious cases — it does not stop, say,
`sh -c ...`, `find /`, or a program that reads a path from stdin or an env
var. `run_command` always requires explicit approval (`y`, or
`--approve-commands` headless) and is refused outright in plan mode; that
approval gate, not path confinement, is what keeps it safe.

Use `--demo-workspace DIR` to (re)create `DIR` with a couple of sample files
(`notes.txt`, `main.go`) and use it as the workspace; this is how every
glyph spec and most Go tests start from a known state.

## Headless mode

`--headless -p PROMPT [--json] [--approve-edits] [--approve-commands]` runs
at most one turn with no TTY and exits; edits and commands are denied unless
explicitly approved by flag. `--dump COLSxROWS` additionally (or instead)
prints the resulting UI's JSON dump — the same shape `tuimark dump --format
json` produces — and suppresses the per-step event trace so the output stays
one JSON value.

## Tests

```
go test ./examples/agent/...
```

covers the tool sandbox (confinement, exact-edit semantics, plan-mode
refusal), the scripted provider's prompt parsing and streaming, the ollama
client against both a purpose-built `httptest` server and the loopback
fixture (streaming content and tool calls), headless end-to-end behavior
(list/edit/approve/deny/plan-mode/--dump/--fixture), and dump checks at 80
and 120 columns (the side panel is present at 120 and absent at 80, with no
error diagnostics either way).

## Glyph specs

```
go build -o bin/tuimark ./cmd/tuimark
go build -o bin/agent   ./examples/agent
glyph run specs/glyphrun/agent_chat.yml \
          specs/glyphrun/agent_edit_approved.yml \
          specs/glyphrun/agent_edit_denied.yml \
          specs/glyphrun/agent_plan_mode.yml \
          specs/glyphrun/agent_resize.yml \
          specs/glyphrun/agent_fixture_ollama.yml --format md
```

Each spec seeds its own `.glyphrun/ws/<name>-ws` demo workspace (git-ignored,
recreated on every run) via `--demo-workspace`, so they do not interfere with
each other or with anything under `--workspace`'s default.
