---
title: TypeScript client
description: "@thelacanians/tuimark (0.4): drive a Tuimark view from a Bun program. spawnHost starts tuimark host, and the client writes data, reads it back, and answers the actions the document fires."
---

# TypeScript client

`@thelacanians/tuimark` (0.4) is the TypeScript client of the [host protocol](/guide/host-protocol). Your Bun program calls `spawnHost`, which starts `tuimark host` with the document. From then on `tuimark` owns the terminal: it lays out, paints, and handles keys. Your program only does three things:

- it writes data into the document with `set` and `batch`;
- it reads data back with `get`;
- it answers the actions the document fires.

It is a parent library, not a second runtime. Everything you see is still drawn by `tuimark`.

```sh
bun add @thelacanians/tuimark
```

The package also has a `tuimark` command (`npx @thelacanians/tuimark new my-app`). Its version is the Tuimark release it runs: it downloads that release's binary once, checks it against a checksum built into the package, and caches it. `TUIMARK_BINARY` points it at a binary you installed yourself.

::: info Not on npm yet?
If `bun add` cannot find the package yet (its first release needs a Tuimark release that ships raw binaries), install the copy in a clone of the repository: `bun add @thelacanians/tuimark@file:/path/to/tuimark/sdk/ts`.
:::

- The package is ESM, written in TypeScript, and has no runtime dependencies.
- The client runs on Bun 1.4.2 or later; Node is not supported for it. The `tuimark` command works on Node 18 or later, and on Bun.
- It speaks `protocol: 1`.
- It runs the `tuimark` binary of its own release: `TUIMARK_BINARY`, else the cache, else a checked download. To use another binary, pass `bin`.

::: tip Start from a template
`tuimark new my-app --ts` writes a working project with an `app.tui`, a `theme.tcss`, a `sample.json`, an `AGENTS.md` for your coding agent, and a `parent.ts` built on this package. See [Dumps and tools](/guide/tools).
:::

## Example

```ts
import { spawnHost } from "@thelacanians/tuimark";

const ui = await spawnHost("app.tui", { data: "sample.json", replyTimeout: "5s" });

ui.on("open", async (ev) => {
  // ev: {seq, action, source, keys, value}
  await ui.set("detail", await load(ev.keys.item));
});
ui.on("bye", () => "quit"); // resolving to "quit" ends the session

const exit = await ui.exited; // {reason, signal?, error?, code}
process.exit(exit.code);
```

While the view runs, your program must not read or write the terminal, because `tuimark` owns fds 0, 1, and 2:

- do not touch `process.stdin`;
- print only after `ui.exited` settles;
- if your program has its own full-screen UI, unmount it before `spawnHost` and mount it again after `exited`.

## `spawnHost(file, options?)`

It runs `tuimark host FILE`, waits for its `ready` message, and resolves with a `Host`.

It rejects with `HostError` in two cases: the child cannot be started, or it exits before `ready`, for example because `FILE` does not load or stdin is not a terminal. In the second case the child prints its reason on stderr, and `error.code` is its exit code.

| Option | Meaning |
|---|---|
| `data` | The document's data, bound at the root, like `--data`. A string is the path of a JSON file. An object is written to a temporary file (mode 0600), which is removed as soon as the child is up. |
| `theme` | `"dark"` or `"light"`, like `--theme`. |
| `replyTimeout` | Like `--reply-timeout`: a duration from `100ms` to `1m` (`"5s"`), or a number of milliseconds. |
| `bin` | The executable. Defaults to the binary of this package's release (`TUIMARK_BINARY`, the cache, or a checked download). |
| `handlers` | Handlers registered before the first event can arrive. |
| `onError` | `(ev, err) => unknown`. It receives an error a handler throws, and the session goes on. See [Replies](#replies). |
| `onProtocolError` | `(msg) => void`. It receives each `error` message that belongs to no pending request: reply timeouts, late replies, and failed writes sent without an `id`. |
| `exitWithChild` | Call `process.exit` with the child's code once the child has exited. Default `false`. |
| `cwd`, `env` | The child's working directory and environment. |

## `Host`

| Member | |
|---|---|
| `ready` | The `ready` message: `protocol`, `version`, `cols`, `rows`, `theme`, `catalog`, `diagnostics`. |
| `pid` | The child's process id. |
| `on(action, handler)` | Registers the handler of one action. A later call for the same action replaces it. Returns the host. |
| `set(path, value)` | `Set`. Resolves with the ack. |
| `batch(writes)` | `Batch`, all or nothing. `writes` is `[{path, value}, …]`. |
| `get(path)` | `Get`. Resolves with `{found, value}`. |
| `exited` | Settles when the child has exited. See [Ending](#ending). |
| `close()` | Sends `SIGTERM`, then `SIGKILL` after 2 seconds. It never rejects, and calling it twice is safe. |

`set`, `batch`, and `get` reject with a `RequestError` when the child refuses them (the ack is in `error.ack`). When the child exits first, they reject with a `HostError`.

Writes made before a handler returns are applied before its reply, so the frame drawn after the handler already shows them. `set`, `batch`, and `get` also work while an event is waiting for its reply, so a handler can read and write state first.

A message of 64 MiB or more is refused with a `RangeError` before it is sent, because `tuimark host` ends the session on such a line.

## Replies

Every event waits for a reply, and the client sends it when the handler settles:

- **Resolving to `"quit"`** replies `quit: true`, which ends the session with reason `quit`.
- **Resolving to anything else** replies plainly, and the view goes on.
- **Throwing** replies with the error's message, which ends the session with reason `error`. With `onError`, the error goes to that function instead and the session survives. This is what `OnError` does for a [Go host](/guide/go-api).
- **An action with no handler** is answered at once.

An event is `{seq, action, source, keys, value}`:

- `keys` is `{alias: key}` for a list or table row, and `{}` otherwise.
- `value` is the action's value (an input's text, a checked array), or `null`.

## Ending

`exited` settles once the child has exited:

- It resolves with `{reason, signal?, error?, code}`. `reason` is `quit`, `eof`, `signal`, or `error`. `code` is the child's exit code: 0 for `quit` and `eof`, 1 for `error`, and 128 plus the signal number for `signal`.
- It rejects with `ProtocolVersionError` when `ready` named a protocol other than 1.
- It rejects with `HostError` when the child exited without an `exit` message. Treat that as an error.

The client follows every [rule for parents](/guide/host-protocol#rules-for-parents):

- It forwards `SIGTERM` and `SIGHUP` to the child from `spawnHost` until the child exits, 1.5 seconds after the parent received the signal and only if the child is still running then. A signal sent to the whole process group (closing the terminal window, systemd's `KillMode=control-group`) reaches the child from the kernel too, and a second signal would kill it before it writes `exit`; the child's own grace is 1 second, so a child that got the signal has gone by then, and `exited` resolves with its `exit` message. A signal sent only to the parent (`kill PID`, a test harness) is forwarded after the 1.5 seconds. If the same signal reaches the parent again during the wait, it is forwarded at once.
- It ends the child with `SIGTERM` first, and with `SIGKILL` only after 2 seconds.
- It never touches the terminal.

## Testing a parent

You can test your handlers without a terminal. `tuimark play FILE --format json` prints the same event records the client hands to your handlers (`{action, source, keys, value}`, plus a `step`). Write a handler against that shape, call it directly in a test, and keep one or two end-to-end checks for a real terminal.

## See also

- [Host protocol](/guide/host-protocol): the wire protocol this package speaks, for a parent in another language.
- [Go API](/guide/go-api): `Run`, for a host written in Go.
