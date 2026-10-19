import type { Ack } from "./types.ts";

/**
 * The session failed: the child could not be started, ended without an
 * `exit` message (SPEC §19.1, "Rules for parents"), or the transport broke.
 */
export class HostError extends Error {
  override name = "HostError";
  /** The child's exit code, when it had exited. */
  code?: number;

  constructor(message: string, options?: { code?: number; cause?: unknown }) {
    super(message, options?.cause === undefined ? undefined : { cause: options.cause });
    if (options?.code !== undefined) this.code = options.code;
  }
}

/** `ready` named a protocol this package does not speak (it speaks 1). */
export class ProtocolVersionError extends HostError {
  override name = "ProtocolVersionError";
  readonly protocol: unknown;

  constructor(protocol: unknown, options?: { code?: number }) {
    super(
      `tuimark host speaks protocol ${JSON.stringify(protocol)}; this @thelacanians/tuimark speaks protocol 1`,
      options,
    );
    this.protocol = protocol;
  }
}

/** A `set`, `batch`, or `get` the child refused, or answered with an `error`. */
export class RequestError extends HostError {
  override name = "RequestError";
  /** The ack, when the child sent one with `ok: false`. */
  readonly ack?: Ack;

  constructor(message: string, ack?: Ack) {
    super(message);
    if (ack) this.ack = ack;
  }
}
