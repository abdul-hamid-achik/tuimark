import { closeSync } from "node:fs";
import { mkdtemp, rm, writeFile } from "node:fs/promises";
import { constants as osConstants, tmpdir } from "node:os";
import { join } from "node:path";
import { BinaryError, resolveBinary } from "./binary.ts";
import { HostError, ProtocolVersionError, RequestError } from "./errors.ts";
import { readLines } from "./lines.ts";
import { FdWriter } from "./sink.ts";
import type {
  Ack,
  GetResult,
  HostEvent,
  HostExit,
  HostHandler,
  Ready,
  SpawnHostOptions,
  Write,
} from "./types.ts";

/** The protocol this package speaks (SPEC §19.1). */
const PROTOCOL = 1;

/** SPEC §19.1, "Line bound": a line of this many bytes or more ends the session. */
const MAX_LINE_BYTES = 64 * 1024 * 1024;

/** SPEC §19.1, "Ending the child": SIGTERM, wait at least 2 seconds, then SIGKILL. */
const KILL_AFTER_MS = 2000;

/** How long the last of fd 4 may take to drain once the child has exited. */
const DRAIN_GRACE_MS = 1000;

/**
 * How long a SIGTERM or SIGHUP the parent received waits before it is
 * forwarded. A signal sent to the whole process group (a closing terminal
 * window, systemd's `KillMode=control-group`) reaches the child from the
 * kernel as well, and the child treats a second signal as fatal; its own
 * grace is 1 second, so a child that got the signal has exited by now.
 */
const FORWARD_GRACE_MS = 1500;
let forwardGraceMs = FORWARD_GRACE_MS;

/**
 * @internal A test hook (not exported from the package entry point): changes
 * the forwarding delay for hosts created from now on; no argument restores it.
 */
export function setForwardGraceMs(ms?: number): void {
  forwardGraceMs = ms ?? FORWARD_GRACE_MS;
}

type Subprocess = Bun.Subprocess<"inherit", "inherit", "inherit">;
type Message = Record<string, unknown> & { type: string };

interface Pending {
  resolve: (ack: Ack) => void;
  reject: (error: Error) => void;
}

function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function messageOf(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

function exitCodeOf(proc: Subprocess): number {
  if (proc.exitCode !== null) return proc.exitCode;
  const signal = proc.signalCode;
  const number = typeof signal === "number" ? signal : signal ? osConstants.signals[signal] : undefined;
  return number === undefined ? 1 : 128 + number;
}

/**
 * A running `tuimark host` child. Get one from {@link spawnHost}.
 *
 * It is a parent of the SPEC §19.1 protocol and nothing more: the Go runtime
 * lays out, paints, and owns the terminal. While the child runs, this object
 * never reads or writes fds 0, 1, or 2.
 */
export class Host {
  /**
   * Settles once the child has exited and its `exit` message was read. It
   * rejects with {@link ProtocolVersionError} when `ready` named a protocol
   * other than 1 (the child is stopped), and with {@link HostError} when the
   * child exited without an `exit` message (SPEC §19.1: treat that as `error`).
   */
  readonly exited: Promise<HostExit>;

  readonly #proc: Subprocess;
  readonly #out: FdWriter;
  readonly #stopReading = new AbortController();
  readonly #forwardTimers = new Map<NodeJS.Signals, ReturnType<typeof setTimeout>>();
  readonly #handlers = new Map<string, HostHandler>();
  readonly #onError: SpawnHostOptions["onError"];
  readonly #onProtocolError: SpawnHostOptions["onProtocolError"];
  readonly #pending = new Map<number, Pending>();
  readonly #signalListeners: Array<[NodeJS.Signals, () => void]> = [];
  readonly #dropTemp: () => Promise<void>;
  readonly #resolveReady: (ready: Ready) => void;
  readonly #rejectReady: (error: Error) => void;
  readonly #resolveExit: (exit: HostExit) => void;
  readonly #rejectExit: (error: Error) => void;
  readonly #readyPromise: Promise<Ready>;

  #ready: Ready | undefined;
  #exitMessage: Omit<HostExit, "code"> | undefined;
  #failure: Error | undefined;
  #finished = false;
  #termSent = false;
  #killTimer: ReturnType<typeof setTimeout> | undefined;
  // Everything after `ready` waits until the caller of `spawnHost` has had a
  // turn, so that an event in the same read as `ready` finds its handler.
  #gateOpen = false;
  #held: Array<() => void> = [];
  #nextId = 1;

  private constructor(
    proc: Subprocess,
    fd3: number,
    fd4: number,
    options: SpawnHostOptions,
    dropTemp: () => Promise<void>,
  ) {
    this.#proc = proc;
    this.#dropTemp = dropTemp;
    this.#onError = options.onError;
    this.#onProtocolError = options.onProtocolError;
    for (const [action, handler] of Object.entries(options.handlers ?? {})) {
      this.#handlers.set(action, handler);
    }

    // Bun gives fd 3 and fd 4 as raw descriptor numbers, the ends of Unix
    // socket pairs (SPEC §19.1, "Parents by language"): write fd 3 with a
    // FileSink (Bun.file(fd).writer(), see FdWriter) and read fd 4 with
    // Bun.file(fd).stream(); the client closes both when it is done with
    // them. readSync, createReadStream, createWriteStream, and net.Socket do
    // not work on them. A write after the child is gone fails with EPIPE; the
    // exit is handled by the watcher below, so that error is dropped.
    this.#out = new FdWriter(fd3);

    let resolveReady!: (ready: Ready) => void;
    let rejectReady!: (error: Error) => void;
    this.#readyPromise = new Promise<Ready>((resolve, reject) => {
      resolveReady = resolve;
      rejectReady = reject;
    });
    this.#resolveReady = resolveReady;
    this.#rejectReady = rejectReady;
    // `ready` is awaited by spawnHost; this keeps a rejection nobody else
    // observes from being reported as unhandled.
    this.#readyPromise.catch(() => {});

    let resolveExit!: (exit: HostExit) => void;
    let rejectExit!: (error: Error) => void;
    this.exited = new Promise<HostExit>((resolve, reject) => {
      resolveExit = resolve;
      rejectExit = reject;
    });
    this.#resolveExit = resolveExit;
    this.#rejectExit = rejectExit;
    // A caller that only awaits `close()` must not see an unhandled rejection.
    this.exited.catch(() => {});

    // SPEC §19.1, "Signals to the parent": forward SIGTERM and SIGHUP to the
    // child. Waiting for it and exiting with its code is the caller's choice
    // (`exitWithChild`, or `await host.exited`).
    //
    // The forward waits `forwardGraceMs` and only goes out if the child is
    // still running then: a signal aimed at the whole process group has
    // reached the child already, and a second one would kill it before it
    // writes `exit`. A second signal of the same kind during the wait means the
    // user insists; it is forwarded at once.
    for (const signal of ["SIGTERM", "SIGHUP"] as const) {
      const forward = () => {
        clearTimeout(this.#forwardTimers.get(signal));
        this.#forwardTimers.delete(signal);
        if (!this.#finished && proc.exitCode === null && proc.signalCode === null) proc.kill(signal);
      };
      const listener = () => {
        if (this.#finished) return;
        if (this.#forwardTimers.has(signal)) {
          forward();
          return;
        }
        this.#forwardTimers.set(signal, setTimeout(forward, forwardGraceMs));
      };
      process.on(signal, listener);
      this.#signalListeners.push([signal, listener]);
    }

    let transportError: unknown;
    const readerDone = readLines(
      Bun.file(fd4).stream(),
      (line) => this.#onLine(line),
      this.#stopReading.signal,
    )
      .catch((error: unknown) => {
        transportError = error;
      })
      .finally(() => {
        // Reading never closes the descriptor; this does, once, when it settles.
        try {
          closeSync(fd4);
        } catch {
          // already closed
        }
      });
    void this.#watch(readerDone, () => transportError);
  }

  /** @internal Use {@link spawnHost}. */
  static async spawn(file: string, options: SpawnHostOptions = {}): Promise<Host> {
    let bin = options.bin;
    if (bin === undefined) {
      try {
        bin = await resolveBinary({ log: (line) => process.stderr.write(`${line}\n`) });
      } catch (error) {
        const reason = error instanceof BinaryError ? error.message : messageOf(error);
        throw new HostError(`cannot find the tuimark binary: ${reason}`, { cause: error });
      }
    }
    const args = ["host", file];

    let tempDir: string | undefined;
    const dropTemp = async () => {
      const dir = tempDir;
      tempDir = undefined;
      if (dir) await rm(dir, { recursive: true, force: true });
    };

    try {
      if (typeof options.data === "string") {
        args.push("--data", options.data);
      } else if (options.data !== undefined) {
        tempDir = await mkdtemp(join(tmpdir(), "tuimark-host-"));
        const path = join(tempDir, "data.json");
        await writeFile(path, JSON.stringify(options.data), { mode: 0o600 });
        args.push("--data", path);
      }
      if (options.theme) args.push("--theme", options.theme);
      if (options.replyTimeout !== undefined) {
        const timeout =
          typeof options.replyTimeout === "number" ? `${options.replyTimeout}ms` : options.replyTimeout;
        args.push("--reply-timeout", timeout);
      }

      let proc: Subprocess;
      try {
        // The terminal stays on fds 0-2, untouched; fd 3 (parent -> child)
        // and fd 4 (child -> parent) are the two extra pipes of §19.1.
        proc = Bun.spawn([bin, ...args], {
          cwd: options.cwd,
          env: options.env,
          stdio: ["inherit", "inherit", "inherit", "pipe", "pipe"],
        } as Bun.SpawnOptions.OptionsObject<"inherit", "inherit", "inherit">) as Subprocess;
      } catch (error) {
        throw new HostError(`cannot start "${bin}": ${messageOf(error)}`, { cause: error });
      }

      const stdio = (proc as unknown as { stdio: unknown[] }).stdio;
      const fd3 = stdio[3];
      const fd4 = stdio[4];
      if (typeof fd3 !== "number" || typeof fd4 !== "number") {
        proc.kill("SIGKILL");
        throw new HostError("Bun did not give the parent descriptors for fd 3 and fd 4");
      }

      const host = new Host(proc, fd3, fd4, options, dropTemp);
      try {
        await host.#readyPromise;
      } finally {
        await dropTemp(); // the child has loaded its data by the time it says ready
      }
      // After `spawnHost` returns, the caller gets a turn before anything else
      // that arrived with `ready` is delivered.
      setTimeout(() => host.#openGate(), 0);
      if (options.exitWithChild) host.#exitWithChild();
      return host;
    } catch (error) {
      await dropTemp();
      throw error;
    }
  }

  /** The child's `ready` message: size, theme, version, catalog, diagnostics. */
  get ready(): Ready {
    return this.#ready!;
  }

  /** The child's process id. */
  get pid(): number {
    return this.#proc.pid;
  }

  /**
   * Registers the handler of one action; a later call for the same action
   * replaces it. The handler's settling is the reply (SPEC §19.2): resolving
   * to `"quit"` replies `quit: true`, anything else replies plainly, and a
   * throw replies with its message as `error` (or goes to `onError`). An
   * action with no handler is answered at once.
   */
  on(action: string, handler: HostHandler): this {
    this.#handlers.set(action, handler);
    return this;
  }

  /**
   * `Set(path, value)`: resolves with the child's ack. Rejects with
   * {@link RequestError} when the child refuses the write.
   */
  async set(path: string, value: unknown): Promise<Ack> {
    return this.#request("set", { path, value: value === undefined ? null : value });
  }

  /** `Batch`: all the writes or none (SPEC §18.1). Resolves with the ack. */
  async batch(writes: readonly Write[]): Promise<Ack> {
    return this.#request("batch", {
      writes: writes.map((w) => ({ path: w.path, value: w.value === undefined ? null : w.value })),
    });
  }

  /** `Get(path)`: `{found, value}`. */
  async get(path: string): Promise<GetResult> {
    const ack = await this.#request("get", { path });
    return ack.found ? { found: true, value: ack.value } : { found: false };
  }

  /**
   * Ends the child as SPEC §19.1 asks: `SIGTERM`, then `SIGKILL` if it is
   * still running 2 seconds later. Resolves once it has exited, whatever the
   * outcome; read {@link Host.exited} for that.
   */
  async close(): Promise<void> {
    this.#terminate();
    await this.exited.then(
      () => {},
      () => {},
    );
  }

  // ---- reading fd 4 ------------------------------------------------------

  #onLine(line: string): void {
    let message: unknown;
    try {
      message = JSON.parse(line);
    } catch {
      return; // a conforming child never writes this, only one that died mid-line
    }
    if (!isObject(message) || typeof message.type !== "string") return;
    const msg = message as Message;

    if (this.#ready === undefined) {
      if (msg.type !== "ready") {
        this.#fail(new HostError(`expected "ready" first on fd 4, got "${msg.type}"`));
        this.#rejectReady(this.#failure!);
        return;
      }
      const ready = msg as unknown as Ready;
      this.#ready = ready;
      if (ready.protocol !== PROTOCOL) {
        // SPEC §19.1: a parent must check `protocol` and stop on a value it
        // does not know. spawnHost still returns (the child did get up), but
        // `exited` rejects and every request fails.
        this.#fail(new ProtocolVersionError(ready.protocol));
      }
      this.#resolveReady(ready);
      return;
    }
    if (this.#failure) return;
    this.#deliver(() => this.#handle(msg));
  }

  #deliver(run: () => void): void {
    if (this.#gateOpen || this.#ready === undefined) run();
    else this.#held.push(run);
  }

  #openGate(): void {
    this.#gateOpen = true;
    const held = this.#held;
    this.#held = [];
    for (const run of held) run();
  }

  #handle(msg: Message): void {
    switch (msg.type) {
      case "event":
        void this.#dispatch(msg);
        return;
      case "ack": {
        const ack = msg as unknown as Ack;
        const pending = this.#take(ack.id);
        if (!pending) return;
        if (ack.ok === false) {
          pending.reject(new RequestError(ack.error ?? "the child refused the request", ack));
        } else {
          pending.resolve(ack);
        }
        return;
      }
      case "error": {
        const id = msg.id;
        const text = typeof msg.error === "string" ? msg.error : "tuimark host error";
        // Malformed input never gets an ack, only an error that echoes its id.
        const pending = id === undefined ? undefined : this.#take(id);
        if (pending) {
          pending.reject(new RequestError(text));
          return;
        }
        this.#onProtocolError?.({
          error: text,
          ...(typeof id === "number" || typeof id === "string" ? { id } : {}),
          ...(typeof msg.seq === "number" ? { seq: msg.seq } : {}),
        });
        return;
      }
      case "exit": {
        this.#exitMessage = {
          reason: msg.reason as HostExit["reason"],
          ...(typeof msg.signal === "string" ? { signal: msg.signal } : {}),
          ...(typeof msg.error === "string" ? { error: msg.error } : {}),
        };
        return;
      }
      default:
        return; // "ready" twice, or a type from a newer 1.x child
    }
  }

  #take(id: unknown): Pending | undefined {
    if (typeof id !== "number") return undefined;
    const pending = this.#pending.get(id);
    if (pending) this.#pending.delete(id);
    return pending;
  }

  // ---- events and replies ------------------------------------------------

  async #dispatch(msg: Message): Promise<void> {
    const event: HostEvent = {
      seq: msg.seq as number,
      action: String(msg.action),
      source: typeof msg.source === "string" ? msg.source : "",
      keys: isObject(msg.keys) ? msg.keys : {},
      value: msg.value,
    };
    const reply: Record<string, unknown> = { type: "reply", seq: event.seq };

    const handler = this.#handlers.get(event.action);
    if (handler) {
      try {
        if ((await handler(event)) === "quit") reply.quit = true;
      } catch (error) {
        if (this.#onError) {
          try {
            await this.#onError(event, error);
          } catch (again) {
            reply.error = messageOf(again);
          }
        } else {
          // `error` counts whenever it is present, even as "" (SPEC §19.1).
          reply.error = messageOf(error);
        }
      }
    }

    try {
      this.#write(reply);
    } catch {
      // the child is gone; there is nobody to answer
    }
  }

  // ---- writing fd 3 ------------------------------------------------------

  #write(message: object): void {
    const line = JSON.stringify(message);
    if (Buffer.byteLength(line) >= MAX_LINE_BYTES) {
      // The child would end the session on a line this long (SPEC §19.1).
      throw new RangeError("a line of 64 MiB or more would end the session; send less per message");
    }
    if (this.#finished || this.#out.ended) throw new HostError("the host has exited");
    this.#out.write(`${line}\n`);
  }

  #request(type: string, members: Record<string, unknown>): Promise<Ack> {
    if (this.#failure) return Promise.reject(this.#failure);
    if (this.#finished) return Promise.reject(new HostError("the host has exited"));
    return new Promise<Ack>((resolve, reject) => {
      const id = this.#nextId++;
      try {
        this.#write({ type, ...members, id });
      } catch (error) {
        reject(error);
        return;
      }
      this.#pending.set(id, { resolve, reject });
    });
  }

  // ---- the child's life --------------------------------------------------

  #fail(error: Error): void {
    this.#failure = error;
    this.#terminate();
    for (const [id, pending] of this.#pending) {
      this.#pending.delete(id);
      pending.reject(error);
    }
  }

  #terminate(): void {
    if (this.#termSent || this.#finished) return;
    this.#termSent = true;
    const proc = this.#proc;
    proc.kill("SIGTERM");
    this.#killTimer = setTimeout(() => {
      if (proc.exitCode === null && proc.signalCode === null) proc.kill("SIGKILL");
    }, KILL_AFTER_MS);
  }

  async #watch(readerDone: Promise<void>, transportError: () => unknown): Promise<void> {
    await this.#proc.exited;
    // The child's last lines (`exit` among them) may still be on their way
    // through fd 4; wait for its end, but not for ever.
    let timer: ReturnType<typeof setTimeout> | undefined;
    await Promise.race([
      readerDone,
      new Promise<void>((resolve) => {
        timer = setTimeout(resolve, DRAIN_GRACE_MS);
      }),
    ]);
    clearTimeout(timer);
    // Still reading after the grace (something else holds the child's end of
    // fd 4 open): stop, so that the descriptor can be closed.
    this.#stopReading.abort();
    await Promise.race([readerDone, new Promise<void>((resolve) => setTimeout(resolve, 250).unref())]);
    const code = exitCodeOf(this.#proc);
    this.#deliver(() => this.#finish(code, transportError()));
  }

  #finish(code: number, transportError: unknown): void {
    this.#finished = true;
    clearTimeout(this.#killTimer);
    for (const timer of this.#forwardTimers.values()) clearTimeout(timer);
    this.#forwardTimers.clear();
    for (const [signal, listener] of this.#signalListeners) process.off(signal, listener);
    this.#signalListeners.length = 0;
    // Gives the child end of file on fd 3, if it is still reading, and closes
    // it. `exited` settles after that, so that a caller who awaits it sees no
    // descriptor of this session left open.
    const closed = this.#out.close();
    void this.#dropTemp();

    let error: Error | undefined;
    let exit: HostExit | undefined;
    if (this.#ready === undefined) {
      error = new HostError(`tuimark host exited before it sent "ready" (code ${code})`, { code });
      this.#rejectReady(error);
    } else if (this.#failure) {
      error = this.#failure;
      if (error instanceof HostError && error.code === undefined) error.code = code;
    } else if (this.#exitMessage) {
      exit = { ...this.#exitMessage, code };
    } else {
      error = new HostError(
        `tuimark host exited without an "exit" message (code ${code})` +
          (transportError ? `: reading fd 4 failed: ${messageOf(transportError)}` : ""),
        { code },
      );
    }

    const pendingError = error ?? new HostError("the host has exited", { code });
    for (const [id, pending] of this.#pending) {
      this.#pending.delete(id);
      pending.reject(pendingError);
    }
    if (error) {
      const rejectExit = this.#rejectExit;
      void closed.then(() => rejectExit(error));
    } else if (exit) {
      const resolveExit = this.#resolveExit;
      void closed.then(() => resolveExit(exit));
    }
  }

  #exitWithChild(): void {
    this.exited.then(
      (exit) => process.exit(exit.code),
      (error: unknown) => {
        // The child is gone, so the terminal is the parent's again.
        console.error(`tuimark host: ${messageOf(error)}`);
        const code = error instanceof HostError ? error.code : undefined;
        process.exit(code ? code : 1);
      },
    );
  }
}

/**
 * Starts `tuimark host FILE` and resolves once the child has sent `ready`.
 *
 * ```ts
 * const ui = await spawnHost("app.tui", { data, theme: "dark", replyTimeout: "5s" });
 * ui.on("open", async (ev) => {
 *   await ui.set("detail", await load(ev.keys.item));
 * });
 * const exit = await ui.exited; // {reason, signal?, error?, code}
 * ```
 *
 * It rejects with {@link HostError} when the child cannot be started or ends
 * before it says `ready` (a bad `FILE`, an unusable terminal: the child's
 * own message is on stderr, and its exit code is `error.code`). A `ready`
 * with a protocol other than 1 does not reject here but rejects
 * {@link Host.exited}.
 */
export function spawnHost(file: string, options: SpawnHostOptions = {}): Promise<Host> {
  return Host.spawn(file, options);
}
