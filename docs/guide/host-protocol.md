---
title: Host protocol
description: tuimark host (0.3b) - running a Tuimark document in the terminal for a parent process written in any language, over a JSON line protocol on file descriptors 3 and 4.
---

# Host protocol

The Go API ([`Run`](/guide/go-api#run)) is for a Go host. `tuimark host` (0.3b) is for everyone else: it runs a document in the terminal exactly as `Run()` would, while a **parent** process in any language supplies the data and answers the actions, over a small JSON line protocol. The child lays out, paints, and decides everything; the parent only speaks the protocol.

This is different from shelling out to `dump`/`validate`/`play`: those are one-shot batch calls. `tuimark host` is interactive — it holds the terminal open for a whole session, the way `Run()` does.

## When to reach for it

- You want a Tuimark view in front of a program written in TypeScript, Python, Rust, or anything else, without a second host runtime for that language. (The SPEC only ever recognizes one host implementation, the Go one; a parent over this protocol is not a second host — it lays out and paints nothing itself.)
- You want a thin process boundary between "what the terminal shows" and "what has the data and the business logic" — the parent can be the same process that already talks to your database or your API.

If your host is already Go, use [`Run`](/guide/go-api#run) directly; it is simpler and has no protocol to speak.

## Starting it

```sh
tuimark host FILE [--data FILE.json] [--theme dark|light] [--reply-timeout 5s]
```

- `--data` binds a JSON file as the store, exactly as `dump --data` does; the parent typically overwrites and extends it with `set`/`batch` messages once the session is running.
- `--theme` is applied as `Set("@theme", …)` before the first frame; `TUIMARK_THEME` still wins over it, as it does everywhere else `Run()` reads the environment.
- `--reply-timeout` (default `5s`, from `100ms` to `1m`) bounds how long a host action waits for the parent's reply before the session moves on.

The transport is **POSIX only**: on Windows, `host` is a usage error, because passing extra file descriptors to a child process needs `os/exec`'s `ExtraFiles` (Unix) or an equivalent Windows has no counterpart for here.

## Transport: fd 0-2 and fd 3/4

The child inherits the terminal unchanged, on file descriptors 0, 1, and 2 — it reads keys and the mouse from stdin and writes frames to stdout exactly as `Run()` does. The protocol never touches those three.

The parent passes two more descriptors when it spawns the child:

- **fd 3**, which the child reads: parent → child.
- **fd 4**, which the child writes: child → parent.

Every message is one UTF-8 JSON object per line (`\n`-terminated), with a `type` member. `tuimark host` checks that fd 3 and fd 4 are pipes or sockets, open for reading and writing respectively, before it loads the document or touches the terminal; a parent that passes nothing there gets a usage error, not a hang.

In Go, pass the two ends in `exec.Cmd.ExtraFiles`. In Bun (see the excerpt below), `stdio: ["inherit", "inherit", "inherit", "pipe", "pipe"]` gives the parent `proc.stdio[3]`/`proc.stdio[4]` as the raw descriptor numbers.

## Messages

### Parent → child (fd 3)

| `type` | Members | Effect |
|---|---|---|
| `set`, `bind` | `path`, `value`, optional `id` | `Set(path, value)`. `bind` is an alias of `set`. |
| `batch` | `writes: [{"path","value"}…]`, optional `id` | `Batch`, all or nothing |
| `get` | `id`, `path` | `Get(path)` |
| `reply` | `seq`, optional `quit: true`, optional `error: "msg"` | answers the `event` numbered `seq` |

A message that carries an `id` (a number or string you choose) gets an `ack` back; one without `id` gets nothing on success, and an `error` on failure, so no failure is silent. That includes a `reply`: one with an `id` that answers the waiting event gets `{"type":"ack","id":…,"ok":true}` before the loop resumes (so it comes before the `exit` a `quit` reply leads to), and every `error` about a reply (late, for an unknown `seq`, or malformed) carries the reply's `id` when it had one, next to its `seq`. A `seq` is any JSON integer (`1`, `1.0`, and `1e0` all answer event 1); a `seq` echoed in an `error` is written exactly as you sent it. Messages are applied in the order they arrive from one reader, so writes sent before a `reply` are applied before that reply is processed — the frame drawn after the handler returns shows them.

### Child → parent (fd 4)

| `type` | Members | When |
|---|---|---|
| `ready` | `protocol`, `version`, `cols`, `rows`, `theme`, `catalog`, `diagnostics` | once, always first |
| `event` | `seq`, `action`, `source`, `keys`, `value` | a host action fired |
| `ack` | `id`, `ok`, optional `error`, `found`, `value` | answers a message that had an `id` |
| `error` | `error`, optional `id`, optional `seq` | a malformed message, a failed write without `id`, a reply timeout, or a late/unknown reply |
| `exit` | `reason` (`quit`, `eof`, `signal`, `error`), optional `signal`, optional `error` | once, always last |

`ready` is the handshake: `protocol` is `1` (check it — a parent must stop on a value it does not know), `version` is the bare runtime version, `cols`/`rows`/`theme` are the session's starting size and resolved theme, `catalog` lists every action the document names (the same shape as `Catalog()`), and `diagnostics` holds the document's static problems.

Every non-built-in action in the catalog becomes an `event`: the child writes it with a fresh `seq` (1, 2, … per session) and waits for a `reply` with that `seq`, up to `--reply-timeout`. While it waits, keys keep queuing (dispatched after the handler returns) and `set`/`batch`/`get` keep being served, so the parent can read and write state before replying. `{"type":"reply","seq":N}` alone resumes the loop; `"quit": true` ends the session with reason `quit`; `"error": "msg"` ends it with reason `error` (which wins when both are present). A reply that never arrives becomes an `error` on fd 4 and the loop moves on — the parent lost that one event, but the session keeps going.

When the session ends, `exit` is the last message: `reason` is `"quit"`, `"eof"` (the terminal's input ended, or the parent closed fd 3), `"signal"` (with `signal`: `"SIGTERM"`, `"SIGHUP"`, or `"SIGINT"`), or `"error"` (with `error`). The process exits `0` for `quit`/`eof`, `1` for `error`, and `128 + n` for `signal`.

**Backpressure.** The child writes fd 4 through a queue of at most 1024 messages. A parent that stops reading, or a write that fails, ends the session with reason `error` — so a parent that stops reading holds the loop for at most the reply timeout, and the terminal is always restored, never held hostage by a stuck pipe.

## A minimal Bun parent

`examples/host-ts/parent.ts` is a complete, runnable example; the [repository's copy](https://github.com/abdul-hamid-achik/tuimark/tree/main/examples/host-ts) has the full error handling this excerpt leaves out. It drives `examples/host-ts/view.tui`, a document with one action, `inc`, and answers it by writing the new count back to the store:

```ts
import { createWriteStream } from "node:fs";

const proc = Bun.spawn({
  cmd: ["tuimark", "host", "view.tui", "--data", "sample.json"],
  stdio: ["inherit", "inherit", "inherit", "pipe", "pipe"],
});

const toChild = createWriteStream("", { fd: proc.stdio[3] as number });
const send = (msg: object) => toChild.write(`${JSON.stringify(msg)}\n`);

let count = 0; // the parent owns the model; the view only shows it

async function readMessages() {
  const reader = Bun.file(proc.stdio[4] as number).stream().getReader();
  const decoder = new TextDecoder();
  let buf = "";
  for (;;) {
    const { value, done } = await reader.read();
    if (done) break;
    buf += decoder.decode(value, { stream: true });
    for (let nl = buf.indexOf("\n"); nl >= 0; nl = buf.indexOf("\n")) {
      const msg = JSON.parse(buf.slice(0, nl));
      buf = buf.slice(nl + 1);
      if (msg.type === "event" && msg.action === "inc") {
        count++;
        send({ type: "set", path: "count", value: count }); // applied before the reply below
      }
      if (msg.type === "event") send({ type: "reply", seq: msg.seq }); // every event gets a reply
    }
  }
}

for (const sig of ["SIGTERM", "SIGHUP"] as const) {
  process.on(sig, () => proc.kill(sig));
}

await readMessages();
process.exit(await proc.exited);
```

`fs.readSync`/`fs.createReadStream` fail on the raw socket-pair descriptors Bun hands back for fd 3/4 (`EAGAIN`); read fd 4 with `Bun.file(fd).stream()` and write fd 3 with `fs.createWriteStream("", { fd })`, whose `.end()` gives the child end of file on fd 3.

## Rules for parents

Follow these and a parent behaves correctly under every ending the protocol has:

- **Never touch the terminal.** While the child runs, do not read stdin (`process.stdin` in Bun/Node reads keys the child is trying to read) and do not write stdout — the child owns fds 0 and 1 completely. A parent with its own full-screen UI unmounts it first and remounts it only after `exit`.
- **Forward `SIGTERM` and `SIGHUP`** to the child, wait for its exit, and exit with its code. A parent that dies on the signal instead would orphan the child.
- **End the child with `SIGTERM`, then `SIGKILL` after 2 seconds** if it has not exited — never `SIGKILL` first. `Run()`'s own signal handling restores the terminal within a 1-second grace period; killing it outright skips that and can leave the terminal in raw mode.
- **Check `protocol` in `ready`**, and treat a child that exits without ever sending `exit` as an error — a well-behaved child always sends it last.

## Testing a parent without a terminal

You don't need a pty to test how your parent reacts to events: `tuimark play FILE --format json` prints the same event records (`{step, action, source, keys, value}` minus `step`) that the host protocol's `event` messages carry. Write your parent's event handler against that shape, and test it directly; save the pty-and-real-child testing for one or two end-to-end checks.

## See also

- [Go API](/guide/go-api#run) — `Run()`, for a Go host that doesn't need a second process.
- [MCP server](/guide/mcp) — for driving the *authoring* loop (`validate`/`dump`/`play`) from an agent, not running a live session.
