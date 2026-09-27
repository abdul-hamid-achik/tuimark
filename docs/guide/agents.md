---
title: Working with coding agents
description: How a coding agent builds terminal UIs with Tuimark - the AGENTS.md briefing from tuimark agents, the validate, dump and play loop, and the rules that keep it on track.
---

# Working with coding agents

Tuimark was designed for a coding agent to write. An agent is good at editing text and reading structured output, and cannot look at a terminal. So in Tuimark the UI is text, and every frame can be printed as JSON: the agent edits files, renders them, reads back exactly what is on the screen, and repeats.

## The briefing: AGENTS.md

`tuimark agents` prints a complete briefing for an agent: what to edit and what not to, the loop to follow, the tags and the attributes each takes, every CSS property and its values, the key tokens, every diagnostic code, and the behavior worth knowing. It is generated from the runtime's own catalogs, so it always matches the binary that printed it.

Put it where your agent looks for instructions:

```sh
tuimark agents > AGENTS.md
```

Regenerate it when you upgrade Tuimark. The [reference](/reference/) pages on this site are generated from the same output.

## The loop

The whole method fits in four commands:

```sh
tuimark validate app.tui --data sample.json
tuimark dump app.tui --data sample.json --cols 80  --rows 24 --format json
tuimark dump app.tui --data sample.json --cols 120 --rows 24 --format json
tuimark play app.tui --data sample.json --input "tab down enter" --styles --format json
```

1. **Edit** the `.tui`, the `.tcss`, and the sample data.
2. **Validate.** Fix every error; read the warnings. Each diagnostic has a code, a position, and usually says what to write instead.
3. **Dump** at 80 and 120 columns (and 40 if it has to fit a narrow pane). Read `grid` for what the screen shows and `nodes` for where each element landed, and check that `errors` is empty.
4. **Play** the interactions: read `events` to confirm the right actions fired with the right `keys` and `value`, and `styles` to confirm that focus and selection actually change how cells look.

Stop when the dumps match the intent at every width and nothing is reported. Then a person, or a host program, can run it for real.

### Reading a dump

| Question | Where to look |
|---|---|
| Does it render at all? | `ok` is `true` and `errors` is empty |
| Is the text right? | `grid`, one string per row |
| Is this panel the right size, in the right place? | the node's `x`, `y`, `w`, `h` in `nodes` |
| Is the list scrolled to the selection? | the list's `scroll` and the `selected` item |
| Does focus start in the right place? | `focus` |
| Did a conditional class apply? | the node's `classes` |
| Is the focused row highlighted? | `play … --styles`: the row's spans have `"a": ["reverse"]` |
| Why is this cell this color? | `tuimark inspect app.tui --at X,Y` |

## Rules that keep an agent on track

The briefing states them, and they are worth repeating in your own instructions:

- **Edit only view files**: `*.tui`, `*.tcss`, and sample data. The view never needs Go.
- **Never invent a tag or a property.** The vocabulary is closed; an invented one is a diagnostic, not a feature. Check the [tags](/reference/tags) and [properties](/reference/css) instead.
- **No logic in markup.** If the view needs a computed value, it goes in the data, and the host computes it.
- **No terminal UI framework code.** No hand-written layout, widths, padding, or truncation in the host; Tuimark does all of that from the document.
- **Name actions; do not implement them in markup.** `on:select="open"` is a name, and the host decides what it means.

## A prompt that works

> Build the view for a deploy dashboard in `ui/app.tui` and `ui/theme.tcss`, with realistic data in `ui/sample.json`: a pipeline list on the left, a log panel on the right, a status bar, and a confirm modal for deploying. Use `<tui version="2">`. Read `AGENTS.md` first. Loop with `tuimark validate` and `tuimark dump --format json` at 80 and 120 columns until there are no diagnostics, and check the keymap with `tuimark play`. Do not write any Go. When you are done, list the action names the view fires.

## Handing off to the host

When the view is done, the host needs to know what to handle. `tuimark validate app.tui --json` includes the action catalog: every name, where it is used, and which are built in. Save the names the host implements to `actions.json`, and from then on:

```sh
tuimark validate app.tui --catalog actions.json
```

reports any action the view fires that the host does not know (`B004`), which catches a renamed action on either side. In Go, `ui.Catalog()` gives the same list; a [test](/guide/testing#go-tests) can require a handler for every name.

## Keeping it honest

- Commit goldens for the sizes you checked ([testing](/guide/testing#golden-dumps)): the next agent that edits the view gets a failing test and a diff, not a surprise.
- Keep `sample.json` realistic, with the long names, empty lists, and wide characters real data will have.
- Prefer `version="2"`: `when` over the focus chain, the built-in `move-*` and `check-*` actions, and `<hints>` mean far less host code, and less for an agent to get wrong.
