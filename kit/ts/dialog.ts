// dialog: open kit/components/confirm.tui and route its answer.
//
// Copy this file into your project. It depends on @tuimark/host (types) and
// on route.ts (instance): copy that file with this one.
//
// The component reads confirm.open, confirm.title and confirm.message and the
// host writes confirm.open and confirm.result ("confirmed" or "cancelled").
// Its actions are dialog_confirm and dialog_cancel, fired by whichever button
// was pressed; one handler of each serves every dialog, told apart by the
// instance of the event's source (confirm__yes: confirm). A dialog is opened by
// your own code, from your own action, with confirm().
import type { Host, HostEvent } from "@tuimark/host";
import { instance } from "./route.ts";

type Ui = Pick<Host, "batch" | "on">;

/** A question to put to the user. */
export interface Request {
  title: string;
  body: string;
  /** The yes answer. */
  onConfirm?: () => unknown;
  /** The no answer, esc included. */
  onCancel?: () => unknown;
}

/**
 * The dialogs of one app. confirm(inst, title, body, onConfirm) opens a k-dialog
 * instance, register() installs the dialog_confirm and dialog_cancel handlers
 * (confirmed / cancelled), and the answer is routed by instance.
 */
export function createDialogs(ui: Ui) {
  const pending = new Map<string, Request>(); // by instance

  /** Opens the dialog `inst` with a question. Opening an instance that is already open replaces its question. */
  async function open(inst: string, r: Request): Promise<void> {
    pending.set(inst, r);
    await ui.batch([
      { path: `${inst}.title`, value: r.title },
      { path: `${inst}.message`, value: r.body },
      { path: `${inst}.result`, value: "none" },
      { path: `${inst}.open`, value: true },
    ]);
  }

  /** Opens the dialog `inst` with a title and a message, and runs `onConfirm` if the user confirms. Cancelling just closes it. */
  function confirm(inst: string, title: string, body: string, onConfirm?: () => unknown): Promise<void> {
    return open(inst, { title, body, onConfirm });
  }

  async function answer(ev: HostEvent, result: "confirmed" | "cancelled"): Promise<void> {
    const inst = instance(ev.source);
    const r = pending.get(inst);
    pending.delete(inst);
    // Close first, so a callback that opens the dialog again is not undone.
    await ui.batch([
      { path: `${inst}.open`, value: false },
      { path: `${inst}.result`, value: result },
    ]);
    if (!r) return; // the dialog was already answered
    await (result === "confirmed" ? r.onConfirm : r.onCancel)?.();
  }

  /** The handler of dialog_confirm. */
  const confirmed = (ev: HostEvent) => answer(ev, "confirmed");
  /** The handler of dialog_cancel. */
  const cancelled = (ev: HostEvent) => answer(ev, "cancelled");

  /** Registers confirmed and cancelled as the handlers of dialog_confirm and dialog_cancel. */
  function register(): void {
    ui.on("dialog_confirm", confirmed);
    ui.on("dialog_cancel", cancelled);
  }

  return { confirm, open, confirmed, cancelled, register };
}

export type Dialogs = ReturnType<typeof createDialogs>;
