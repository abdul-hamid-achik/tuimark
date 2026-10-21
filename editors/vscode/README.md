# Tuimark for VS Code

Editor support for [Tuimark](https://github.com/abdul-hamid-achik/tuimark), the view language for terminals. You design a terminal UI by editing two kinds of text files, and this extension makes them pleasant to edit:

- `.tui`: the document, an XML subset (`<tui>`, `<screen>`, `<list>`, `<keymap>`, ...).
- `.tcss`: the stylesheet, a CSS subset for terminal cells.

## Features

**Syntax highlighting** for both languages, with no binary needed.

- In `.tui`: the tags of the catalog (including the 0.4b `include`, `fragment`, `template`, `use`, `checkbox`, and `select`, and the 0.4c `slot`), shown apart from names that are not in it; attributes (the `with="path as alias"` of a `use` and its `template=`, `single`, `indeterminate`); `on:*` action attributes and the built-in actions; `class:NAME` guards; `{path}` interpolations and `{$name}` prop references in text and in `title`, `placeholder`, `label`, and `short` (a brace pair that holds more than a path is marked, as `tuimark validate` reports it as `V003`); `each="path as alias"`; `if="!path"`; key tokens in `keys`; `#id` targets in `to`, `when`, and `focus`; entities; and the TCSS inside `<style>` and `style=""`.
- In `.tcss`: selectors (`#id`, `.class`, tags, `>`, lists), the pseudo-classes `:focus`, `:selected`, `:disabled`, `:empty`, `:checked`, `:focus-within`, and `:root`; properties and their keywords; `$tokens` and `var(--x)`; colors (`#rgb`, `#rrggbb`, the 16 ANSI names); `@media` with `max-cols`, `min-cols`, `max-rows`, `min-rows`, and `theme`; comments.

A property, pseudo-class, media feature, or tag that Tuimark does not have is marked as illegal, because the vocabulary is closed.

**Snippets** for a document skeleton, a list with `each` and `key`, a table with columns, tabs, a modal, keymap rows, the form controls (`checkbox`, `select`, `radio`, `spinner`), composition (`include`, `fragment`, `template`, `use`, and a template with props and a slot), and the theme tokens. Type `tui`, `list`, `table`, `tabs`, `modal`, `bind`, `template`, `use`, or `root` and press Tab.

**Language server.** When the `tuimark` binary is installed, the extension starts `tuimark lsp` over stdio. It gives the same diagnostics as `tuimark validate` as you type, completion, hover, go to definition, and formatting with `tuimark fmt`.

> `tuimark lsp` may ship in a `0.4.N` release after 0.4.0. With a `tuimark` that does not have it, or with no `tuimark` on your machine at all, the extension shows one warning and keeps the highlighting and the snippets working.

## Requirements

Nothing, for highlighting and snippets. For the language server, install `tuimark` (see the [repository](https://github.com/abdul-hamid-achik/tuimark#install)) and make sure `tuimark version` works in a terminal. The extension runs `tuimark version` first and starts the server only if it exits with status 0 and prints a `tuimark ...` version line.

## Restricted Mode

The extension supports [Workspace Trust](https://code.visualstudio.com/docs/editor/workspace-trust) in a limited way. In a workspace you have not trusted (Restricted Mode), highlighting and snippets keep working, but the language server does not start: a workspace can set `tuimark.path` in `.vscode/settings.json`, which chooses the program the extension runs. The workspace value of `tuimark.path` is ignored until you trust the workspace, and the server starts as soon as you do, with no reload.

## Settings

| Setting | Default | |
|---|---|---|
| `tuimark.path` | `tuimark` | Path to the `tuimark` binary. The default is looked up on `PATH`. A relative path such as `./bin/tuimark` is resolved against the first workspace folder. Restricted in untrusted workspaces. Changing it restarts the server. |
| `tuimark.trace.server` | `off` | `messages` or `verbose` logs the traffic with `tuimark lsp` in the *Tuimark* output channel. |

## Commands

- **Tuimark: Restart Language Server** restarts `tuimark lsp`, for example after you install or upgrade the binary. It does nothing in a workspace that is not trusted.

## The loop this is for

```
tuimark validate app.tui
tuimark dump app.tui --data sample.json --cols 80 --rows 24 --format json
tuimark run app.tui
```

The editor shows the diagnostics of the first command in place; the others are what you run in a terminal.

## Development

The extension lives in `editors/vscode/` of the Tuimark repository and uses Bun.

```
cd editors/vscode
bun install --frozen-lockfile
bun run build      # type-check, then bundle src/extension.ts into out/
bun run test       # grammar tests (vscode-tmgrammar-test) and manifest tests
bun run package    # produces tuimark-<version>.vsix
```

The grammars are tested on annotated copies of the repository's fixtures under `test/`, and every `.tui` and `.tcss` of `examples/` is tokenized to check that nothing the runtime accepts is marked illegal.

## License

MIT
