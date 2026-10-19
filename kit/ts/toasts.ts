// toasts: a queue of toasts for kit/components/toast.tui, with expiry.
//
// Copy this file into your project. It depends only on @tuimark/host (types).
//
// The component is one row: toast.tui shows the toast in toast.text while
// toast.show is true, with the level in toast.is_ok, toast.is_warn and
// toast.is_danger (none of them is info). This helper keeps a queue behind
// that row. The first toast shows at once and later ones wait their turn; each
// shows for its time to live, and tick() moves to the next. The TS host has no
// Every, so start() runs tick() on a timer. Esc, bound to toast_dismiss in the
// component, drops the one on screen (register()).
import type { Host } from "@tuimark/host";

type Ui = Pick<Host, "batch" | "on">;

export type ToastKind = "info" | "ok" | "warn" | "danger";

export interface ToastOptions {
  /** The store object the component reads. Default "toast". */
  path?: string;
  /** The most toasts held, the one on screen included. Default 5. */
  max?: number;
  /** The time to live, in milliseconds, of a toast pushed without one. Default 3000. */
  ttl?: number;
  /** The clock, for tests. Default Date.now. */
  now?: () => number;
  /** Called with an error a timer-driven tick hit. Default: ignore. */
  onError?: (error: unknown) => void;
}

interface Item {
  kind: ToastKind;
  text: string;
  ttl: number;
  deadline: number; // set when it goes on screen
}

export function createToasts(ui: Ui, options: ToastOptions = {}) {
  const path = options.path ?? "toast";
  const max = Math.max(1, options.max ?? 5);
  const defaultTtl = options.ttl ?? 3000;
  const now = options.now ?? Date.now;
  const queue: Item[] = []; // queue[0] is on screen

  /** Writes the head of the queue (or hides the row) and starts the head's clock. */
  async function showHead(): Promise<void> {
    const head = queue[0];
    if (head) head.deadline = now() + head.ttl;
    await ui.batch([
      { path: `${path}.text`, value: head?.text ?? "" },
      { path: `${path}.is_ok`, value: head?.kind === "ok" },
      { path: `${path}.is_warn`, value: head?.kind === "warn" },
      { path: `${path}.is_danger`, value: head?.kind === "danger" },
      { path: `${path}.show`, value: head !== undefined },
    ]);
  }

  /**
   * Queues a toast. `ttl` (milliseconds) is how long it stays on screen once
   * it is shown. If the queue is full, the oldest toast that is waiting is
   * dropped; if only one toast is allowed, the new one replaces it.
   */
  async function push(kind: ToastKind, text: string, ttl = 0): Promise<void> {
    if (!["info", "ok", "warn", "danger"].includes(kind)) {
      throw new Error(`kit: toast kind "${String(kind)}" is not info, ok, warn or danger`);
    }
    const item: Item = { kind, text, ttl: ttl > 0 ? ttl : defaultTtl, deadline: 0 };
    if (queue.length < max) queue.push(item);
    else if (queue.length > 1) {
      queue.splice(1, 1); // drop the oldest waiting
      queue.push(item);
    } else queue[0] = item; // max is 1: replace
    if (queue.length === 1 || max === 1) await showHead();
  }

  /** Drops the toast on screen; the next one, if any, takes its place. */
  async function dismiss(): Promise<void> {
    if (queue.length === 0) return;
    queue.shift();
    await showHead();
  }

  /** Drops the toast on screen when its time is up and shows the next. Writes only when something changes. */
  async function tick(at: number = now()): Promise<void> {
    const head = queue[0];
    if (!head || at < head.deadline) return;
    queue.shift();
    await showHead();
  }

  /** Calls tick() every `everyMs`. Returns the function that stops it. */
  function start(everyMs = 500): () => void {
    const id = setInterval(() => {
      tick().catch((error: unknown) => options.onError?.(error));
    }, everyMs);
    return () => clearInterval(id);
  }

  /** Registers the handler of toast_dismiss. */
  function register(): void {
    ui.on("toast_dismiss", () => dismiss());
  }

  return { push, dismiss, tick, start, register, size: () => queue.length };
}

export type Toasts = ReturnType<typeof createToasts>;
