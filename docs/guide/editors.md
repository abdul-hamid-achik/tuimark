---
title: Editors
description: Editor support for .tui and .tcss (0.4) - the VS Code extension (highlighting, snippets, and the tuimark lsp client) and how to use tuimark lsp from Neovim, Helix, or any other LSP client.
---

# Editors

Tuimark files are plain text, so any editor can open them. Since 0.4 there are two kinds of help on top of that:

- **The VS Code extension.** It adds highlighting and snippets for `.tui` and `.tcss`, and starts the language server.
- **`tuimark lsp`.** This is the language server, built into the `tuimark` binary. It shows `validate`'s diagnostics as you type, and adds completion, hover, go to definition, and formatting. It works with any editor that speaks the Language Server Protocol.

## VS Code

Install **Tuimark** from the Visual Studio Marketplace or from Open VSX: search for "Tuimark" in the Extensions view.

**Highlighting** works with nothing else installed:

- tags, attributes, and `on:*` actions;
- `{path}` interpolations, `each="path as alias"`, and key tokens;
- the TCSS inside `<style>` and `style=""`;
- in `.tcss`: selectors, pseudo-classes, properties and their keywords, `$tokens`, colors, and `@media`.

The vocabulary is closed, so a tag, property, pseudo-class, or media feature that Tuimark does not have is marked as illegal while you type. A brace pair that holds more than one path is marked too, since `validate` reports it as `V003`.

**Snippets.** Type `tui`, `list`, `table`, `tabs`, `modal`, `bind`, or `root` and press Tab.

**Language server.** When `tuimark` is on your `PATH`, the extension starts `tuimark lsp`. If it cannot start the server, either because there is no `tuimark` or because it is a version without `lsp`, it shows one warning, and highlighting and snippets keep working.

| Setting | Default | |
|---|---|---|
| `tuimark.path` | `tuimark` | The `tuimark` binary. The default is looked up on `PATH`. Changing it restarts the server. |
| `tuimark.trace.server` | `off` | Set it to `messages` or `verbose` to log the traffic with the server in the *Tuimark* output channel. |

The command **Tuimark: Restart Language Server** restarts the server, for example after you upgrade `tuimark`.

## What the language server does

`tuimark lsp` runs the same checks as `tuimark validate`, with the same codes and the same messages, on the text in your editor, saved or not.

- **Diagnostics.**
  - A problem in a stylesheet is shown in the stylesheet, even when the document that loads it is the file you are editing.
  - When the document's directory holds a `sample.json`, the data checks (`B001`–`B003`, `B008`–`B010`) run against it, as with `validate --data sample.json`. Without one, they are skipped, as with `validate` alone.
- **Completion.** Tags, attributes, attribute values, `#id` targets, actions, key tokens, and paths from `sample.json`. In a `.tcss`, it completes properties, keywords, `$tokens`, pseudo-classes, and the ids and classes of the documents that load the stylesheet. It offers only what your document's `version` accepts.
- **Hover.** The reference text of the tag, attribute, property, keyword, or diagnostic code under the cursor.
- **Go to definition.** From an `#id` to its element, from a `.class` to every element that has it, from `<style src>` to the file, and from a `$token` to its declarations.
- **Formatting.** It formats a `.tui` with `tuimark fmt`. A `.tcss` has no formatter.

## Other editors

Point your editor's LSP client at `tuimark lsp`, a stdio server, for the file types `.tui` and `.tcss`.

**Neovim** (0.11 or later):

```lua
vim.filetype.add({ extension = { tui = "tui", tcss = "tcss" } })

vim.lsp.config("tuimark", {
  cmd = { "tuimark", "lsp" },
  filetypes = { "tui", "tcss" },
  root_markers = { "AGENTS.md", ".git" },
})
vim.lsp.enable("tuimark")
```

**Helix** (`languages.toml`):

```toml
[language-server.tuimark]
command = "tuimark"
args = ["lsp"]

[[language]]
name = "tui"
scope = "text.tui"
file-types = ["tui"]
roots = []
language-servers = ["tuimark"]

[[language]]
name = "tcss"
scope = "source.tcss"
file-types = ["tcss"]
roots = []
language-servers = ["tuimark"]
```

These two get diagnostics, completion, and the rest from the server, but no highlighting, since the TextMate grammars ship only with the VS Code extension.

## See also

- [Dumps and tools](/guide/tools): `validate`, `dump`, `play`, and the other commands the editor complements.
- [Working with agents](/guide/agents): the loop a coding agent runs, which the editor shows you as you type.
