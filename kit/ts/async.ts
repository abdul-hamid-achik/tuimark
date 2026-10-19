// async: run work off the handler and write its state to the store.
//
// Copy this file into your project. It depends only on @tuimark/host (types).
//
// A handler's settling is its reply, and the terminal waits for it (up to the
// reply timeout), so the slow part of an action (a request, a scan) should not
// run inside it. load() starts it and keeps three paths under the one you
// name, so the markup can show a "loading" line, an error and the data with
// ordinary `if` and {path}:
//
//   <path>.loading  true while the work runs
//   <path>.error    "" or the error's message
//   <path>.data     the result (kept, not cleared, while a reload runs)
import type { Host } from "@tuimark/host";

type Ui = Pick<Host, "batch" | "get">;

export interface Task {
  /** Settles when the run has finished: it wrote its result, or it was canceled or superseded and dropped it. */
  done: Promise<void>;
  /**
   * Aborts the signal given to the work. Its result is dropped; <path>.loading
   * goes back to false, and the old data and error stay. A run that a later
   * load() superseded writes nothing: the later load owns <path>.loading.
   */
  cancel(): void;
}

export interface LoaderOptions {
  /** Called with an error the final write hit (the store refused the value, or the connection ended). Default: ignore. */
  onError?: (error: unknown) => void;
}

/**
 * A loader for one app: load(path, fn) runs `fn` and keeps <path>.loading,
 * <path>.error and <path>.data. `task.done` never rejects; failures of the
 * final write go to options.onError.
 */
export function createLoader(ui: Ui, options: LoaderOptions = {}) {
  const current = new Map<string, AbortController>();

  /**
   * Sets <path>.loading to true and <path>.error to "", runs `fn`, then writes
   * <path>.data (or <path>.error) and <path>.loading false in one batch. If
   * load() is called again for the same path while `fn` runs, the first run is
   * aborted and its result dropped, so a slow old request never overwrites a
   * newer one. The value must be JSON-shaped.
   */
  async function load(path: string, fn: (signal: AbortSignal) => Promise<unknown>): Promise<Task> {
    current.get(path)?.abort();
    const ctl = new AbortController();
    current.set(path, ctl);
    const hasData = (await ui.get(`${path}.data`)).found;
    await ui.batch([
      { path: `${path}.loading`, value: true },
      { path: `${path}.error`, value: "" },
      ...(hasData ? [] : [{ path: `${path}.data`, value: null }]),
    ]);
    const done = (async () => {
      let value: unknown;
      let message = "";
      try {
        value = await fn(ctl.signal);
      } catch (error) {
        message = error instanceof Error ? error.message : String(error);
      }
      const mine = current.get(path) === ctl;
      if (mine) current.delete(path);
      if (!mine) return; // superseded by a later load, which owns the paths now
      if (ctl.signal.aborted) {
        // canceled while current: stop the spinner, keep the rest
        await ui.batch([{ path: `${path}.loading`, value: false }]).catch((error: unknown) => options.onError?.(error));
        return;
      }
      try {
        await ui.batch([
          ...(message === "" ? [{ path: `${path}.data`, value: value ?? null }] : []),
          { path: `${path}.error`, value: message },
          { path: `${path}.loading`, value: false },
        ]);
      } catch (error) {
        // The store refused the value: say why, and stop the spinner.
        options.onError?.(error);
        try {
          await ui.batch([
            { path: `${path}.error`, value: error instanceof Error ? error.message : String(error) },
            { path: `${path}.loading`, value: false },
          ]);
        } catch (second) {
          options.onError?.(second);
        }
      }
    })();
    return { done, cancel: () => ctl.abort() };
  }

  return { load };
}

export type Loader = ReturnType<typeof createLoader>;
