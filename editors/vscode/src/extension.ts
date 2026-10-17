import { execFile } from "node:child_process";
import * as path from "node:path";
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
//
// The server is an executable the workspace can influence (tuimark.path may be
// set in .vscode/settings.json), so it only starts in a trusted workspace; in
// Restricted Mode the extension is limited to grammars and snippets.

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
    // Trust granted later in the session: now the server may start.
    vscode.workspace.onDidGrantWorkspaceTrust(async () => {
      await restart();
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

// The command to run, as one value for both the probe and the server. A bare
// name ("tuimark") is looked up on PATH. A relative path with a separator
// ("./bin/tuimark") is resolved once, to an absolute path, against the first
// workspace folder: the language client starts the server with the workspace
// root as its cwd, and the probe would otherwise use the extension host's.
function binaryPath(): string {
  const configured = vscode.workspace.getConfiguration("tuimark").get<string>("path");
  const trimmed = (configured ?? "").trim();
  if (trimmed === "") {
    return "tuimark";
  }
  const hasSeparator = trimmed.includes("/") || trimmed.includes("\\");
  if (!hasSeparator || path.isAbsolute(trimmed)) {
    return trimmed;
  }
  const base = vscode.workspace.workspaceFolders?.[0]?.uri.fsPath ?? process.cwd();
  return path.resolve(base, trimmed);
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

// Runs `tuimark version` and accepts only a real tuimark: exit status 0 and
// output that starts with "tuimark ". "The process ran" is not enough: a
// workspace-chosen tuimark.path such as /bin/sh runs (and fails) on `version`
// and would then be started as `sh lsp`. A missing binary is also the one
// failure vscode-languageclient reports with its own error toast, so it is
// caught here first.
function probeBinary(command: string): Promise<{ ok: true } | { ok: false; reason: string }> {
  return new Promise((resolve) => {
    execFile(command, ["version"], { timeout: PROBE_TIMEOUT_MS, windowsHide: true }, (err, stdout) => {
      if (!err) {
        if (String(stdout).trimStart().startsWith("tuimark ")) {
          resolve({ ok: true });
        } else {
          resolve({
            ok: false,
            reason: `"${command} version" did not print a tuimark version. Is tuimark.path a tuimark binary?`,
          });
        }
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
      resolve({
        ok: false,
        reason: `"${command} version" failed (${code ?? "non-zero exit"}). Is tuimark.path a tuimark binary?`,
      });
    });
  });
}

async function startClient(): Promise<void> {
  if (!vscode.workspace.isTrusted) {
    output?.appendLine(
      "The workspace is not trusted: the language server starts once you trust it. Highlighting and snippets work meanwhile.",
    );
    return;
  }
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
