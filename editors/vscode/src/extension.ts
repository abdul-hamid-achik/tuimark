import { execFile } from "node:child_process";
import * as vscode from "vscode";
import {
  CloseAction,
  ErrorAction,
  LanguageClient,
  type LanguageClientOptions,
  type ServerOptions,
  TransportKind,
} from "vscode-languageclient/node";

// The language server is `tuimark lsp` over stdio (SPEC section 15.12). It may
// ship in a later 0.4.N than the rest of 0.4a, so every failure to reach it
// degrades to "grammars and snippets only" with one non-modal warning.

const CLIENT_ID = "tuimark";
const OUTPUT_NAME = "Tuimark";
// A server that stops sooner than this after it started never got going.
const EARLY_EXIT_MS = 5000;
const PROBE_TIMEOUT_MS = 5000;

// vscode-languageclient shows its own error toast when a server cannot start
// (start() reports "couldn't create connection to server" with 'force'). Until
// the server is running, those errors go to the output channel only: the one
// warning below says what the user needs to know.
class TuimarkClient extends LanguageClient {
  override error(message: string, data?: unknown, showNotification?: boolean | "force"): void {
    super.error(message, data, this.isRunning() ? showNotification : false);
  }
}

let client: LanguageClient | undefined;
let output: vscode.OutputChannel | undefined;
// One warning per activation; a restart (command or setting) re-arms it.
let warned = false;

export async function activate(context: vscode.ExtensionContext): Promise<void> {
  warned = false;
  output = vscode.window.createOutputChannel(OUTPUT_NAME);
  context.subscriptions.push(output);

  context.subscriptions.push(
    vscode.commands.registerCommand("tuimark.restartServer", async () => {
      await restart();
    }),
    vscode.workspace.onDidChangeConfiguration(async (e) => {
      if (e.affectsConfiguration("tuimark.path")) {
        await restart();
      }
    }),
  );

  await startClient();
}

export async function deactivate(): Promise<void> {
  await stopClient();
}

async function restart(): Promise<void> {
  await stopClient();
  warned = false;
  await startClient();
}

async function stopClient(): Promise<void> {
  const c = client;
  client = undefined;
  if (c) {
    try {
      await c.stop();
    } catch {
      // The server may already be gone.
    }
  }
}

function binaryPath(): string {
  const configured = vscode.workspace.getConfiguration("tuimark").get<string>("path");
  const trimmed = (configured ?? "").trim();
  return trimmed === "" ? "tuimark" : trimmed;
}

function warnOnce(message: string): void {
  output?.appendLine(message);
  if (warned) {
    return;
  }
  warned = true;
  // Non-modal and not awaited: the warning never blocks activation.
  void vscode.window
    .showWarningMessage(
      `Tuimark: ${message} Syntax highlighting and snippets still work.`,
      "Open Settings",
    )
    .then((choice) => {
      if (choice === "Open Settings") {
        void vscode.commands.executeCommand("workbench.action.openSettings", "tuimark.path");
      }
    });
}

// Runs `tuimark version` to learn whether the binary exists at all. A missing
// binary is the one failure vscode-languageclient reports with its own error
// toast, so it is caught here first.
function probeBinary(command: string): Promise<{ ok: true } | { ok: false; reason: string }> {
  return new Promise((resolve) => {
    execFile(command, ["version"], { timeout: PROBE_TIMEOUT_MS, windowsHide: true }, (err) => {
      if (!err) {
        resolve({ ok: true });
        return;
      }
      const code = (err as NodeJS.ErrnoException).code;
      if (code === "ENOENT" || code === "EACCES") {
        resolve({
          ok: false,
          reason: `could not run "${command}" (${code}). Install tuimark or set tuimark.path.`,
        });
        return;
      }
      // The binary ran but `version` failed: let the server start decide.
      resolve({ ok: true });
    });
  });
}

async function startClient(): Promise<void> {
  const command = binaryPath();
  const probe = await probeBinary(command);
  if (!probe.ok) {
    warnOnce(`${probe.reason}`);
    return;
  }

  const serverOptions: ServerOptions = {
    run: { command, args: ["lsp"], transport: TransportKind.stdio },
    debug: { command, args: ["lsp"], transport: TransportKind.stdio },
  };

  const startedAt = Date.now();
  const clientOptions: LanguageClientOptions = {
    documentSelector: [
      { scheme: "file", language: "tuimark" },
      { scheme: "file", language: "tcss" },
      { scheme: "untitled", language: "tuimark" },
      { scheme: "untitled", language: "tcss" },
    ],
    outputChannel: output,
    errorHandler: {
      error: () => ({ action: ErrorAction.Continue, handled: true }),
      closed: () => {
        if (Date.now() - startedAt < EARLY_EXIT_MS) {
          warnOnce(
            `"${command} lsp" exited right after starting. This tuimark may predate the language server (it ships in a 0.4.x release).`,
          );
        } else {
          warnOnce(`"${command} lsp" stopped. Run "Tuimark: Restart Language Server" to try again.`);
        }
        return { action: CloseAction.DoNotRestart, handled: true };
      },
    },
    initializationFailedHandler: () => false,
  };

  const next = new TuimarkClient(CLIENT_ID, OUTPUT_NAME, serverOptions, clientOptions);
  client = next;
  try {
    await next.start();
  } catch (err) {
    if (client === next) {
      client = undefined;
    }
    const detail = err instanceof Error ? err.message : String(err);
    warnOnce(`"${command} lsp" did not start (${detail}).`);
  }
}
