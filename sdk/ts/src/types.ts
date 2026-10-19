/** The session's theme, `--theme` and `ready.theme` (SPEC §19.1). */
export type Theme = "dark" | "light";

/** Why a session ended: the `exit` message's `reason` (SPEC §19.1). */
export type ExitReason = "quit" | "eof" | "signal" | "error";

/** One action of the document's `Catalog()`, as `ready` lists it. */
export interface CatalogEntry {
  name: string;
  builtin: boolean;
  sources: string[];
}

/** The `ready` message: the first thing the child writes on fd 4. */
export interface Ready {
  type: "ready";
  /** The protocol version. This package speaks `1`. */
  protocol: number;
  /** The bare runtime version, e.g. `0.4.0`. */
  version: string;
  cols: number;
  rows: number;
  /** The session's effective theme, after the probe resolved `auto`. */
  theme: Theme;
  catalog: CatalogEntry[];
  /** The document's static diagnostics, in the §14 JSON form. */
  diagnostics: unknown[];
}

/** A host action that fired: the §15.4 event record without `step`. */
export interface HostEvent {
  /** 1, 2, … per session. The client echoes it in the reply. */
  seq: number;
  action: string;
  /** The id of the node the action came from, or `""`. */
  source: string;
  /** For a list or table row: `{alias: key}`. `{}` otherwise (never `null`). */
  keys: Record<string, unknown>;
  value: unknown;
}

/**
 * A handler for one action. Its settling is the reply (SPEC §19.2): resolve
 * to `"quit"` to end the session, resolve to anything else to carry on, throw
 * to fail the session (or to reach `onError`, when that option is set).
 */
export type HostHandler = (event: HostEvent) => unknown;

/** One write of a `batch`. */
export interface Write {
  path: string;
  value: unknown;
}

/** The child's answer to a `set`, `batch`, or `get` that carried an `id`. */
export interface Ack {
  id: number | string;
  ok: boolean;
  error?: string;
  /** `get` only. */
  found?: boolean;
  /** `get` only, when found. */
  value?: unknown;
}

/** What `get` resolves with. `value` is `undefined` unless `found`. */
export interface GetResult {
  found: boolean;
  value?: unknown;
}

/**
 * An `error` message the child sent that no pending request owns: a reply
 * timeout (`seq`), a late or unknown reply, a malformed line, or a failed
 * write without an `id`.
 */
export interface ProtocolErrorMessage {
  error: string;
  id?: number | string;
  seq?: number;
}

/** How the session ended: the `exit` message plus the child's exit code. */
export interface HostExit {
  reason: ExitReason;
  /** `"SIGTERM"`, `"SIGHUP"`, or `"SIGINT"` when `reason` is `"signal"`. */
  signal?: string;
  /** The message when `reason` is `"error"`. */
  error?: string;
  /** The child's exit code; 128 + the signal number if a signal killed it. */
  code: number;
}

export interface SpawnHostOptions {
  /**
   * The document's data, bound at the root as `--data` does. A string is the
   * path of a JSON file. Any other value is written (as JSON, mode 0600) to a
   * temporary file that is removed once the child is up.
   */
  data?: string | Record<string, unknown>;
  /** `--theme`. `TUIMARK_THEME` still beats it (SPEC §19.1). */
  theme?: Theme;
  /**
   * `--reply-timeout`: a Go duration from `100ms` to `1m` (`"5s"`), or a
   * number of milliseconds.
   */
  replyTimeout?: string | number;
  /**
   * The `tuimark` executable. By default the one for this package's release:
   * `TUIMARK_BINARY`, else the cached copy, else a checked download (see
   * `resolveBinary`). Pass a path to use your own build; a bare name such as
   * `"tuimark"` is looked up on `PATH`.
   */
  bin?: string;
  /** Handlers to register before the first event can be delivered. */
  handlers?: Record<string, HostHandler>;
  /**
   * Called when a handler throws. The reply is then plain, so the session
   * survives, as Go's `OnError` lets it. If this throws in turn, the reply
   * carries that error and the session ends. Without it, a throw replies with
   * its message as `error`, which ends the session.
   */
  onError?: (event: HostEvent, error: unknown) => unknown;
  /** Called for each `error` message that no pending request owns. */
  onProtocolError?: (message: ProtocolErrorMessage) => void;
  /**
   * Call `process.exit` with the child's code once it has exited. Off by
   * default: the parent owns its process.
   */
  exitWithChild?: boolean;
  /** The child's working directory. */
  cwd?: string;
  /** The child's environment. Defaults to this process's. */
  env?: Record<string, string | undefined>;
}
