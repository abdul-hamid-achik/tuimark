---
title: Keymap, actions and focus
description: How keys, clicks and focus become named actions in Tuimark - the keymap, when selectors over the focus chain, built-in actions, key dispatch, event payloads and the mouse.
---

# Keymap, actions and focus

Markup never runs code. When something happens (a key, a click, a cursor move, a submitted input), the runtime fires an **action**: a name, with a small payload that says where it came from. The host program registers handlers for the names it cares about. A few names are built in.

## Actions are names

Widgets name actions in `on:*` attributes:

```tui
<list id="inbox" each="tickets as t" key="t.id" bind="selected" on:select="open">…</list>
<input id="query" bind="query" on:change="filter" on:submit="search"/>
<button id="deploy" label="Deploy" on:click="ask_deploy"/>
```

| Attribute | On | Fires when |
|---|---|---|
| `on:select` | `list`, `table`, `tabs` | the cursor moves to another row, or another tab becomes active |
| `on:change` | `input`; `list`, `table` with `checked` | the text changes; the set of checked rows changes |
| `on:submit` | `input` | `enter` is pressed in it |
| `on:click` | any node, `button` above all | `enter` or `space` on it while focused, or a mouse click |
| `on:focus` | any focusable node | it receives focus |
| `on:open`, `on:close` | `modal` | it opens or closes |
| `on:escape` | `modal` | `esc` is pressed while it is the top modal |

An action name is a plain identifier (`open`, `ask_deploy`). A name with no handler does nothing, so a view can be written and tested before its host exists; pass `--catalog actions.json` to `validate` to have unknown names reported (`B004`).

## The keymap

Global keys live in one `<keymap>`. Each `<bind>` row maps one or more keys to an action:

```tui
<keymap>
  <bind keys="q,ctrl+c" action="quit"/>
  <bind keys="/" action="focus" to="#query" label="search"/>
  <bind keys="enter" action="open" when="#inbox:focus" label="open"/>
  <bind keys="d" action="delete" when="#inbox:focus"/>
</keymap>
```

- `keys` is a comma-separated list of [key tokens](/reference/keys): `q`, `enter`, `ctrl+a`, `shift+tab`, `/`. Nothing is implicit: if you want `j` and `k` to move a list, bind them.
- `when` is a selector that must match for the row to apply. It is how a key means different things in different places.
- Rows are tried in document order, and the first match wins, so put specific rows before general ones.
- `label` (and `keycap`, the text shown instead of the key) make the row a key hint that [`<hints>`](/guide/widgets#hints) displays (version 2).

A document may hold **several `<keymap>` elements**, in every version: their rows form one keymap, in document order (the first keymap's rows, then the second's). It is a way to group rows without repeating a `when` on each one: in `version="3"`, `<keymap when="SEL">` gives every row inside it that `when`, unless a row writes its own, which replaces it outright rather than combining with it (`when=""` on a row, or on the `<keymap>` itself, means no `when`).

```tui
<keymap when="#procs:focus">
  <bind keys="j" action="move-next" label="down"/>
  <bind keys="k" action="move-prev" label="up"/>
  <bind keys="space" action="check-toggle" label="mark"/>
</keymap>
<keymap when="#kill">
  <bind keys="y" action="confirm_kill" label="yes"/>
  <bind keys="n,esc" action="cancel_kill" label="no"/>
</keymap>
```

### when and the focus chain

In a `version="2"` document, `when` matches if its selector matches the focused node **or any ancestor** up to the screen; with nothing focused, it matches against the top open modal and then the screen. So:

- `when="#procs:focus"`: only while the table `procs` itself has focus;
- `when="#processes"`: while focus is anywhere inside the tab `processes`;
- `when="#kill"`: while focus is inside the modal `kill`;
- `when="#app"`: while focus is in the main view and no modal has it, when `#app` is the screen's first child and the modals are its siblings.

In a `version="1"` document, `when` matches the focused node only.

## Built-in actions

Two built-ins work everywhere:

| Action | Does |
|---|---|
| `quit` | stops the app. With no `quit` handler registered, `Run` returns; with one, it returns when the handler returns `ErrQuit`. |
| `focus` | moves focus to the node `to="#id"` names |

Version 2 adds built-ins for the common widget moves, so no host code is needed for them. They are allowed only in keymap rows, and act on the node `to` names, or else on the focused node:

| Action | On a list or table | On tabs | On a scrolling viewport |
|---|---|---|---|
| `move-next`, `move-prev` | cursor down or up one row | next or previous tab, wrapping | scroll one row |
| `move-first`, `move-last` | first or last row | first or last tab | to the top or the bottom |
| `move-page-down`, `move-page-up` | a page of rows | not applicable | a page |
| `check-toggle` | check or uncheck the cursor row | | |
| `check-all`, `check-none` | check every row, or none | | |
| `switch-to` | | activate the tab `to` names | |

`switch-to` also switches to another screen when `to` names one.

A built-in row only matches when its target can take it: `move-next` needs a list with rows, `check-all` needs a list with `checked`, and so on. When it cannot, the key falls through to the next row, so a global `<bind keys="ctrl+a" action="check-all"/>` never steals `ctrl+a` from an input. A row that matches takes its key even if nothing changes (`move-next` on the last row). The events these actions cause, such as `on:select` or `on:change`, fire as if the user had moved the cursor.

```tui
<bind keys="j" action="move-next" when="#procs:focus"/>
<bind keys="k" action="move-prev" when="#procs:focus"/>
<bind keys="space" action="check-toggle" when="#procs:focus" label="mark"/>
<bind keys="ctrl+a" action="check-all" when="#procs:focus" keycap="^A" label="all"/>
<bind keys="1" action="switch-to" to="#overview" when="#app" keycap="1-9" label="tabs"/>
```

## Focus

One node has focus at a time, or none.

- `input`, `list`, `button`, and `modal` (and in version 2 `table`) can take focus. Any other node can with `focusable="true"`; a `tabs` needs it to take `left` and `right`.
- A focusable node needs an `id` (`V012`).
- `tab` and `shift+tab` cycle through focusable nodes in document order, skipping hidden and disabled ones.
- `<screen focus="#id">` picks the first focused node; otherwise it is the first focusable one.
- An open modal traps focus: `tab` cycles inside it, and a request to focus a node outside it does nothing. When it closes, focus returns to where it was.
- The host moves focus with `Set("@focus", "#id")`, and a keymap row with `action="focus" to="#id"`.

## How a key is handled

The same procedure serves the live terminal, `tuimark play`, and `<hints>`. For each key:

1. **`esc` and a modal.** If the key is `esc` and the top open modal has `on:escape`, that fires and the key is used up.
2. **The focused widget.** A focused, enabled widget takes its own keys:
   - an `input` takes printable keys, `space`, `backspace`, `ctrl+h`, `ctrl+u`, `left`, `right`, `home`, `end`, `ctrl+a`, `ctrl+e`, and `enter` when it has `on:submit`;
   - a `list` or `table` with rows takes `up`, `down`, `home`, `end`, `pgup`, and `pgdn`;
   - a focusable `tabs` takes `left` and `right`;
   - another scrolling viewport takes the arrows and the paging keys;
   - any other node with `on:click` takes `enter` and `space`.
3. **The keymap.** The first row whose keys, `when`, and (for a built-in) target all match fires.
4. **The defaults.** `tab` and `shift+tab` move focus, and `ctrl+c` fires `quit`.

Step 2 is why typing `q` into a search box types a `q` instead of quitting, and why `<hints>` hides the `q quit` hint while the box has focus.

In version 2, each key sees the frame the previous key left, even when several arrive in one read (a fast typist, a paste): a `when` or a class guard that the first key changed applies to the second.

## Events

A handler receives an `Event`:

| Field | Holds |
|---|---|
| `Action` | the action name |
| `Source` | the `id` of the node that fired it |
| `Keys` | the `each` aliases of the row involved, by key: `{"t": "t-12"}` |
| `Value` | the input's text; the new `checked` array; a tab's id; otherwise usually `null` |

A keymap row that fires a host action reports the **focused** node as its source, not the node its `when` matched, and the cursor row of a focused list or table as its `keys`. So `<bind keys="d" action="delete" when="#inbox:focus"/>` tells the handler exactly which ticket to delete.

## The mouse

In version 2, `<tui mouse="…">` turns the mouse on while the flag is truthy. It takes `true`, `false`, a path, or `!path`, and is off by default, because a terminal in mouse mode no longer lets people select text. A setting can switch it with `Set`.

With the mouse on, only the left button and the wheel act:

- **A click** acts when the button is released over the same node it was pressed on. Walking up from the cell: a disabled or hidden node stops it; a tab label activates its tab; a list or table row focuses the widget and moves the cursor there; a table column header with `on:click` fires it (sortable columns); a node with `on:click` takes focus if it can and fires it; any other focusable node takes focus.
- **The wheel** moves a list or table cursor by one row, a tab strip by one tab, or another viewport by one row.
- While a modal is open, only events inside it count.

`tuimark inspect --at X,Y` names the node a click at that cell starts from, and `play` replays clicks with `click:X,Y`, `wheel-up:X,Y`, and `wheel-down:X,Y`.

## Checking interactions without a terminal

`tuimark play` runs the same key handling with no terminal and records every action that fired. Check a keymap before the host exists:

```sh
tuimark play app.tui --data sample.json --input "tab down down enter" --format json
tuimark play app.tui --data sample.json --input "text:deploy enter"
tuimark play app.tui --data sample.json --input "click:12,3 wheel-down:12,5"
```

See [play](/guide/tools#play) for every kind of step.
