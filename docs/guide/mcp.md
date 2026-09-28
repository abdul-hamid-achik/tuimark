---
title: MCP server
description: tuimark mcp (0.3b) - a Model Context Protocol server that gives a coding agent the validate/dump/play authoring loop as five tools, over stdin/stdout.
---

# MCP server

`tuimark mcp` (0.3b) is a [Model Context Protocol](https://modelcontextprotocol.io) server on stdin/stdout. It gives a coding agent the same [authoring loop](/guide/agents#the-loop) — `validate`, `dump`, `play`, plus `inspect` and the `AGENTS.md` briefing — as tool calls, for an agent that reaches Tuimark through an MCP gateway rather than a shell. It is standard library only, JSON-RPC 2.0, one message per line, and every result is exactly what the matching CLI command would print to stdout, byte for byte.

If your agent already has a shell, the [CLI](/guide/tools) and this server do the same work; use whichever your environment gives you. They agree by construction — every tool call runs the same code path as its CLI command.

## Registering it

Point your MCP client at the `tuimark` binary with the `mcp` subcommand and no other arguments:

```json
{
  "mcpServers": {
    "tuimark": {
      "command": "tuimark",
      "args": ["mcp"]
    }
  }
}
```

The exact place this JSON goes depends on your client (a project config file, a global settings file, or a CLI flag); consult its docs for "MCP server" or "stdio server" configuration. The server reads no environment variable and needs no other setup — just a `tuimark` binary on `PATH` (or an absolute `command`).

## The tools

| Tool | Required | Optional | Runs |
|---|---|---|---|
| `tuimark_validate` | `file` | `data`, `theme`, `strict`, `catalog` | `tuimark validate FILE --json` |
| `tuimark_dump` | `file` | `data`, `cols`, `rows`, `theme`, `styles`, `cells`, `strict` | `tuimark dump FILE --format json` |
| `tuimark_play` | `file` | `steps`, `data`, `cols`, `rows`, `theme`, `styles`, `cells`, `frames`, `strict` | `tuimark play FILE --format json` |
| `tuimark_inspect` | `file` | `at`, `id`, `data`, `cols`, `rows`, `theme`, `strict` | `tuimark inspect FILE --json` |
| `tuimark_agents` | — | — | `tuimark agents` |

The list is closed and matches the [loop](/guide/agents#the-loop) on purpose: `fmt` and `test` are left out because they can write files, which this read-only server never does; `ir`, `preview`, `host`, and `version` are left out because they serve runtime debugging, a terminal, or a version string, not authoring.

Every tool argument maps to the flag of the same name (`data` → `--data`, `theme` → `--theme`, and so on); `strict`, `styles`, `cells`, and `frames` become the bare flag when `true` and nothing when `false`. `cols` and `rows` take any JSON number without a fraction (`1e3` is `1000`, `1.0` is `1`), and their exact digits reach `--cols`/`--rows`, so a value out of range gets the CLI's own message for those digits. The `file`, like every other argument, goes through the command's own flag parsing, so the result is what the command line above prints for it. `steps` (default: none, step 0 only) is an array of strings, one [`play` step](/guide/tools#play) each, and — unlike `--input` on the command line — a step in the array may contain spaces (`"text:hello world"`), since there is no shell splitting to worry about.

Every tool is marked `readOnlyHint: true`, and none of them writes a file or keeps state between calls: each call loads its document fresh.

## Reading a result

- When the underlying command would exit `0` (clean) or `2` (validation errors — data, not failure), the tool result is **not** an error, and its text is the command's stdout: the same JSON `tuimark dump --format json` would print, for instance, `"ok": false` included when there were diagnostics. Read `errors`/`ok` in the JSON to see the problems, the same way you would from the CLI.
- When the command would exit `1` (a missing file, a bad flag value), the result has `isError: true`, and its text is the command's stderr line, such as `tuimark: open nope.tui: no such file or directory`.
- **Argument errors** — a required argument missing, an argument the tool does not take, or a value of the wrong JSON type — are also `isError` results, checked before the command runs, so a mistake in the call never starts the CLI: `tuimark: missing argument "file"`, `tuimark: unknown argument "input"` (the CLI's flag name for `steps`, a common slip), `tuimark: cols: want integer`.

An agent following the [authoring loop](/guide/agents#the-loop) reads these the same way it reads CLI output: fix every error `validate` reports, check the `dump` at 80 and 120 columns, and confirm the right action fired with `play`.

## Protocol notes

- `initialize` answers with `protocolVersion` set to whichever of `2025-11-25`, `2025-06-18`, `2025-03-26`, or `2024-11-05` the client asked for (else `2025-11-25`), and `serverInfo.name` is `"tuimark"` with the bare runtime version. It never fails because of the version.
- `tools/list` returns all five tools with no pagination; `ping` answers `{}`.
- The server exits `0` when stdin reaches end of file, and `1` if reading stdin or writing stdout fails.

## See also

- [Working with agents](/guide/agents) — the authoring loop this server exposes as tools, and how to read a dump.
- [Dumps and tools](/guide/tools) — the same commands from a shell.
- [Host protocol](/guide/host-protocol) — for driving a *live* session, not the authoring loop.
