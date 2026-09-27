---
title: Reference
description: Exact tables for the Tuimark CLI, tags and attributes, CSS properties, key tokens and diagnostic codes, generated from the runtime.
---

# Reference

The exact tables. Every page here except this one is generated from the `tuimark` binary itself (its `--help` and the catalogs behind `tuimark agents`), so it matches the runtime you install.

| Page | What it lists |
|---|---|
| [CLI](/reference/cli) | Every command and flag, and the exit codes |
| [Tags and attributes](/reference/tags) | The closed set of tags and the attributes each one takes, in `version="1"` and `version="2"` documents |
| [CSS properties](/reference/css) | Every TCSS property and its values |
| [Key tokens](/reference/keys) | The names a `<bind keys="…">` row accepts |
| [Diagnostic codes](/reference/diagnostics) | Every `V…`, `L…`, and `B…` code and when it is reported |

The same information, in the compact form an agent reads, is one command away:

```sh
tuimark agents
```

For how the pieces fit together, start with the guide: [The .tui language](/guide/language), [TCSS](/guide/tcss), [Layout](/guide/layout).
