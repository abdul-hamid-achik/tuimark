import { closeSync } from "node:fs";

/** How long {@link FdWriter.close} waits for writes still in flight. */
const CLOSE_WAIT_MS = 1000;

const noop = () => {};

/**
 * The parent's end of fd 3: a raw, non-blocking socket-pair descriptor that
 * Bun hands back as a number (SPEC §19.1, "Parents by language").
 *
 * It writes through `Bun.file(fd).writer()`. `fs.createWriteStream("", {fd})`
 * does not work here: on Bun 1.4.2 a write of about 8 KB or more to such a
 * descriptor throws an uncaught TypeError inside the stream, and the stream
 * never moves again.
 *
 * Writes go out in the order they are made, one after the other, and a failed
 * write (EPIPE: the child is gone) is dropped: the exit is handled by whoever
 * watches the process, not here. `FileSink.end()` leaves the descriptor open,
 * so {@link close} closes it, once.
 */
export class FdWriter {
  readonly #fd: number;
  readonly #sink: Bun.FileSink;
  #tail: Promise<void> = Promise.resolve();
  #closing: Promise<void> | undefined;
  #broken = false;

  constructor(fd: number) {
    this.#fd = fd;
    this.#sink = Bun.file(fd).writer();
  }

  /** True once {@link close} was called: nothing more can be written. */
  get ended(): boolean {
    return this.#closing !== undefined;
  }

  /** Queues `text` after everything written before it. */
  write(text: string): void {
    this.#tail = this.#tail.then(() => this.#send(text));
  }

  async #send(text: string): Promise<void> {
    if (this.#broken) return;
    const sink = this.#sink;
    const settling: Array<Promise<unknown>> = [];
    try {
      // Either call may throw (EPIPE) or hand back a promise that rejects
      // later; both are the same event, the child being gone.
      settling.push(Promise.resolve(sink.write(text)));
      settling[0]!.catch(noop);
      settling.push(Promise.resolve(sink.flush()));
      settling[1]!.catch(noop);
    } catch {
      this.#broken = true;
    }
    const results = await Promise.allSettled(settling);
    if (results.some((r) => r.status === "rejected")) this.#broken = true;
  }

  /**
   * Waits for the writes in flight (for a moment, not for ever), ends the sink,
   * and closes the descriptor, which gives the child end of file on fd 3.
   * Safe to call twice.
   */
  close(): Promise<void> {
    this.#closing ??= (async () => {
      await settleWithin(this.#tail, CLOSE_WAIT_MS);
      try {
        await settleWithin(Promise.resolve(this.#sink.end()), CLOSE_WAIT_MS);
      } catch {
        // the descriptor is closed below either way
      }
      try {
        closeSync(this.#fd);
      } catch {
        // already closed
      }
    })();
    return this.#closing;
  }
}

/** Resolves when `promise` settles or after `ms`, whichever comes first. */
async function settleWithin(promise: Promise<unknown>, ms: number): Promise<void> {
  let timer: ReturnType<typeof setTimeout> | undefined;
  await Promise.race([
    promise.then(noop, noop),
    new Promise<void>((resolve) => {
      timer = setTimeout(resolve, ms);
    }),
  ]);
  clearTimeout(timer);
}
