# @thelacanians/tuimark

Tuimark on npm, in one package: the `tuimark` command, and the TypeScript client of Tuimark's host protocol.

```sh
npx @thelacanians/tuimark new my-app      # or: bunx @thelacanians/tuimark new my-app
bun add @thelacanians/tuimark             # the client, in a Bun project
npm install -g @thelacanians/tuimark      # the `tuimark` command everywhere
```

## The `tuimark` command

The package has a `bin` named `tuimark`. It runs the Go binary of the Tuimark release that has the same version as this package (`@thelacanians/tuimark@0.4.3` runs `v0.4.3`), with your arguments, your terminal, and your signals, and exits with its exit code. It needs Node 18 or later, or Bun, and no `postinstall` script (bun and pnpm 10 block those). `tuimark host` also works through it when a parent starts the installed command directly (a global install, or `node_modules/.bin/tuimark`) with its pipes on fd 3 and fd 4; through `npx` or `bunx` the runtime owns those descriptors, so a parent should start `tuimark host` itself, or, from TypeScript, use `spawnHost`, which runs the binary directly.

The binary is found in this order:

1. `TUIMARK_BINARY`, a path to a `tuimark` you installed yourself. It must exist.
2. The cache: `<cache>/tuimark/<version>/<os>_<arch>/tuimark` (`.exe` on Windows), where `<cache>` is `$XDG_CACHE_HOME` or `~/.cache` on Linux, `~/Library/Caches` on macOS, and `%LOCALAPPDATA%` on Windows.
3. A download of `tuimark_<version>_<os>_<arch>` (`.exe` on Windows; `os` is `darwin`, `linux`, or `windows`, `arch` is `amd64` or `arm64`) from the GitHub release `v<version>` of `abdul-hamid-achik/tuimark`. Its SHA-256 must match the table built into this package, or the file is thrown away and never run. A verified binary is written to the cache with an atomic rename, so the download happens once per version, and a cached binary is checked against the same hash on every run.

**Security.** The hashes of the six binaries are written into the package when it is built, from the release's `checksums.txt` (`src/embedded.ts`, see `scripts/embed-checksums.mjs`), so they reach you through npm and not through the GitHub release the binary comes from: a swapped or corrupted release asset is rejected. They are as trustworthy as the package you installed. In a clone of the repository there is no such table, and the release's own `checksums.txt` is used, which only catches corruption.

`TUIMARK_RELEASE_URL` replaces `https://github.com/abdul-hamid-achik/tuimark/releases/download` for mirrors. Then the table is not used: `<url>/v<version>/checksums.txt` and the binary are both read from the mirror, so the checksum only catches a corrupt download, and the binary is cached in a directory of its own (`<os>_<arch>-mirror-<hash of the URL>`), never the one a normal run reads.

Downloads use Node's `fetch`, which ignores `HTTPS_PROXY` unless you set `NODE_USE_ENV_PROXY=1` (Node 24 and later). Behind a proxy that does not work, fetch the binary yourself and set `TUIMARK_BINARY`.

Releases before the one that first shipped raw binaries (0.4.1 and older) only have archives, so this package has no version for them. Offline, or on a platform without a release binary, install `tuimark` another way (`brew install --cask abdul-hamid-achik/tap/tuimark`, or `go install github.com/abdul-hamid-achik/tuimark/cmd/tuimark@latest`) and set `TUIMARK_BINARY`.

## The client

It spawns `tuimark host`, which owns the terminal and runs a `.tui` document, and lets your Bun program drive it: write data into the document, read it back, and answer the actions it fires.

It is a parent library, not a second host. `tuimark` still lays out and paints everything; this package only speaks the protocol and follows its rules for parents.

- ESM, written in TypeScript, no runtime dependencies.
- The client runs on Bun (CI tests it with Bun). Node is not supported for the client; the `tuimark` command above works on both.
- It speaks `protocol: 1`.
- Needs the `tuimark` of the same release, found as above. Pass `bin` to `spawnHost` to use another executable. The [host protocol guide](https://tuimark.dev/guide/host-protocol) describes the wire protocol this package speaks.

## Example

```ts
import { spawnHost } from "@thelacanians/tuimark";

const ui = await spawnHost("app.tui", { data: "sample.json", theme: "dark", replyTimeout: "5s" });

ui.on("open", async (ev) => {
  // ev: {seq, action, source, keys, value}
  await ui.set("detail", await load(ev.keys.item));
});
ui.on("bye", () => "quit"); // resolving to "quit" ends the session

const exit = await ui.exited; // {reason, signal?, error?, code}
process.exit(exit.code);
```

While the child runs, your program must not read or write the terminal: `tuimark` owns fds 0, 1, and 2. Do not touch `process.stdin`, and print only after `ui.exited` settles. A program with its own full-screen UI unmounts it before `spawnHost` and mounts it again after `exited`.

## `spawnHost(file, options?)`

Runs `tuimark host FILE`, waits for its `ready` message, and resolves with a `Host`.

It rejects with `HostError` when the child cannot be started, or exits before it says `ready` (a `FILE` that does not load, stdin that is not a terminal). The child prints its own reason on stderr, and `error.code` is its exit code.

| Option | Type | Meaning |
|---|---|---|
| `data` | `string \| object` | The document's data, bound at the root, as `--data`. A string is the path of a JSON file. An object is written to a temporary file (mode 0600) that is removed as soon as the child is up. |
| `theme` | `"dark" \| "light"` | `--theme`. `TUIMARK_THEME` still beats it. |
| `replyTimeout` | `string \| number` | `--reply-timeout`: a Go duration from `100ms` to `1m` (`"5s"`), or milliseconds. The child's default is `5s`. |
| `bin` | `string` | The executable. Defaults to the binary of this package's release: `TUIMARK_BINARY` (also read from `env`), the cache, or a checked download. A bare name such as `"tuimark"` is looked up on `PATH`. |
| `handlers` | `Record<string, handler>` | Handlers registered before the first event can arrive. |
| `onError` | `(ev, err) => unknown` | See [Replies](#replies). |
| `onProtocolError` | `(msg) => void` | Called with each `error` message that belongs to no pending request: `{error, id?, seq?}`. These are reply timeouts, late or unknown replies, and failed writes sent without an `id`. |
| `exitWithChild` | `boolean` | Call `process.exit` with the child's code once it has exited. Default `false`: the parent owns its process. If the child ended without an `exit` message, it prints the error to stderr and exits with the child's code, or 1 if that was 0. |
| `cwd`, `env` | | The child's working directory and environment. |

## `Host`

| Member | |
|---|---|
| `ready` | The `ready` message: `protocol`, `version`, `cols`, `rows`, `theme`, `catalog`, `diagnostics`. |
| `pid` | The child's process id. |
| `on(action, handler)` | Registers the handler of one action (a later call for the same action replaces it). Returns the host. |
| `set(path, value)` | `Set`. Resolves with the ack. |
| `batch(writes)` | `Batch`, all or nothing. `writes` is `[{path, value}, …]`. Resolves with the ack. |
| `get(path)` | `Get`. Resolves with `{found, value}`; `value` is absent unless `found`. |
| `exited` | Settles when the child has exited. See [Ending](#ending). |
| `close()` | Sends `SIGTERM`, then `SIGKILL` after 2 seconds if the child is still running. Resolves once it has exited, whatever the outcome. Safe to call twice. |

### Messages

`set`, `batch`, and `get` each send their message with a fresh `id` and resolve with the child's `ack`. When the child refuses (`ok: false`), the promise rejects with a `RequestError` carrying the message and the ack (`error.ack`). The same happens when the child answers the `id` with an `error` instead, which is what it does for a malformed message. If the child exits first, pending requests reject with a `HostError`.

A value of `undefined` is sent as `null`. A message whose line is 64 MiB or more is refused with a `RangeError` before it is sent, because the child ends the session on such a line.

Messages are applied in the order they are sent, from one reader. Writes made before a handler returns are applied before its reply, so the frame drawn after the handler shows them. `set`, `batch`, and `get` are served while an event waits for its reply, so a handler can read and write state first.

### Replies

Each event waits for the parent's reply, and the client sends it when the handler settles:

- Resolving to `"quit"` replies `quit: true`, which ends the session with reason `quit`.
- Resolving to anything else replies plainly.
- Throwing replies with the error's message as `error`, which ends the session with reason `error`. An empty message is still sent, as `""`.
- With the `onError(ev, err)` option, a throw goes to that function instead, and the reply is plain, so the session survives, as Go's `OnError` lets it. If `onError` throws in turn, the reply carries that error.
- An event with no handler is answered at once, plainly. Register handlers with the `handlers` option, or call `on` right after `spawnHost` resolves: messages that arrive with `ready` are held until the caller has had a turn, but an event that arrives after an `await` of your own may find no handler.

The reply timeout (`replyTimeout`) is the child's: a handler that takes longer is answered by the child with a plain reply and an `error` message, which reaches `onProtocolError`.

An event is `{seq, action, source, keys, value}`:

| Member | |
|---|---|
| `seq` | 1, 2, … per session. |
| `action` | The action's name. |
| `source` | The id of the node it came from, or `""`. |
| `keys` | For a list or table row, `{alias: key}`. Always an object (`{}` when empty), never `null`. |
| `value` | The action's value (an input's text, a checked array), or `null`. |

### Ending

`exited` settles once the child has exited and its output is read:

- It resolves with `{reason, signal?, error?, code}`. `reason` is `quit`, `eof`, `signal`, or `error`, as in the `exit` message. `signal` names the signal for `signal`, and `error` holds the message for `error`. `code` is the child's exit code (0 for `quit` and `eof`, 1 for `error`, 128 plus the signal number for `signal`).
- It rejects with `ProtocolVersionError` when `ready` named a protocol other than 1. The client stops the child and fails every request. `spawnHost` still resolves in that case, so that you can read `host.ready.protocol`.
- It rejects with `HostError` when the child exited without an `exit` message (`error.code` is its exit code). Treat that as `error`.

A rejection nobody awaits does not crash the process.

## Rules for parents, and what the client does

| [Rule for parents](https://tuimark.dev/guide/host-protocol) | Client |
|---|---|
| Pass fd 3 and fd 4 and read them as Bun needs. | `stdio: ["inherit", "inherit", "inherit", "pipe", "pipe"]`; fd 4 is read with `Bun.file(fd).stream()`, fd 3 is written with `Bun.file(fd).writer()` (a `FileSink`: `fs.createWriteStream` breaks on writes of about 8 KB or more). Both descriptors are closed when the session ends, so the child sees end of file on fd 3 and nothing leaks. |
| Forward `SIGTERM` and `SIGHUP` to the child. | Listeners are installed from `spawnHost` until the child exits, and removed after. While they are installed the signals no longer end your process: wait for `exited`, or set `exitWithChild`, and exit with the child's code. The forward goes out 1.5 seconds after the parent received the signal, and only if the child is still running then (below). |
| Never read or write the terminal while the child runs. | The client never touches fds 0, 1, or 2. |
| End the child with `SIGTERM`, then `SIGKILL` after at least 2 seconds. | `close()`. |
| Check `protocol`; treat an exit without `exit` as `error`. | `exited` rejects in both cases. |

### Signals to the parent

A signal sent to the whole process group (closing the terminal window sends `SIGHUP` to the foreground group; systemd's `KillMode=control-group`; `kill -TERM -PGID`) reaches the child from the kernel and the parent at once. The child treats a second signal as fatal (the default action), so forwarding at once would kill it before it writes `exit`, and `exited` would reject with a `HostError`. The client therefore waits 1.5 seconds after it received `SIGTERM` or `SIGHUP`, and forwards only if the child is still running. The child's own grace is 1 second, so one that got the signal has exited by then and `exited` resolves with its `exit` message (`reason: "signal"`). A signal sent only to the parent's pid (`kill PID`, a test harness) is forwarded once the 1.5 seconds are over, and the child ends then. If the same signal reaches the parent again during the wait, it is forwarded at once: the user insists.

## Errors

All are exported and extend `HostError` (which extends `Error`).

| Class | When |
|---|---|
| `HostError` | The child could not start, ended before `ready`, or ended without an `exit` message; the transport failed. `code` is the child's exit code when known. |
| `ProtocolVersionError` | `ready.protocol` is not 1. `protocol` holds the value. |
| `RequestError` | A `set`, `batch`, or `get` was refused. `ack` holds the ack when there was one. |

## Develop

```sh
bun install
bun test        # against a fake child, test/fake-child.ts
bunx tsc --noEmit
bun run build   # dist/, for publishing
```

The tests need no terminal and no `tuimark` binary (the download tests use a local HTTP server). `bin/tuimark.js` runs the TypeScript source under Bun when `dist/` is not built; after `bun run build` its tests also run it under Node.

The package version is the Tuimark release it drives. `.github/workflows/release.yml` publishes it with npm trusted publishing after GoReleaser has uploaded the release's binaries: the `npm-pack` and `npm-publish` jobs set the version from the `v*` tag, so the `version` in this `package.json` only matters for local runs.

`examples/host-ts` depends on this package (`file:../../sdk/ts`) and records its dev dependencies in its own `bun.lock`. After you change the dependencies in `package.json` here, run `bun install` in `examples/host-ts` and commit its `bun.lock`, or `bun install --frozen-lockfile` fails in CI.

## License

MIT
