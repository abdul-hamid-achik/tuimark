// The client must degrade quietly: with no binary, or a binary whose `lsp`
// exits at once, it shows one non-modal warning and never throws.
import { beforeEach, describe, expect, mock, test } from "bun:test";
import { chmodSync, mkdtempSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

const state = {
  path: "tuimark",
  warnings: [] as string[],
  clients: [] as any[],
  failStart: false,
  closeOnStart: false,
};

mock.module("vscode", () => ({
  window: {
    createOutputChannel: () => ({ appendLine() {}, dispose() {} }),
    showWarningMessage: (msg: string) => {
      state.warnings.push(msg);
      return Promise.resolve(undefined);
    },
  },
  commands: { registerCommand: () => ({ dispose() {} }), executeCommand: () => Promise.resolve() },
  workspace: {
    getConfiguration: () => ({ get: () => state.path }),
    onDidChangeConfiguration: () => ({ dispose() {} }),
  },
}));

mock.module("vscode-languageclient/node", () => ({
  CloseAction: { DoNotRestart: 1, Restart: 2 },
  ErrorAction: { Continue: 1, Shutdown: 2 },
  TransportKind: { stdio: 0 },
  LanguageClient: class {
    opts: any;
    constructor(public id: string, public name: string, public server: any, opts: any) {
      this.opts = opts;
      state.clients.push(this);
    }
    running = false;
    errors: unknown[][] = [];
    isRunning() {
      return this.running;
    }
    error(message: string, data?: unknown, show?: boolean | "force") {
      this.errors.push([message, data, show]);
    }
    async start() {
      if (state.failStart) throw new Error("spawn failed");
      if (state.closeOnStart) this.opts.errorHandler.closed();
    }
    async stop() {}
  },
}));

const ext = await import("../src/extension");
const ctx = () => ({ subscriptions: [] as any[] }) as any;

function fakeTuimark(body: string): string {
  const dir = mkdtempSync(join(tmpdir(), "tuimark-vscode-"));
  const bin = join(dir, "tuimark");
  writeFileSync(bin, `#!/bin/sh\n${body}\n`);
  chmodSync(bin, 0o755);
  return bin;
}

beforeEach(() => {
  state.warnings = [];
  state.clients = [];
  state.failStart = false;
  state.closeOnStart = false;
});

describe("activate", () => {
  test("a missing binary gives one warning and starts no client", async () => {
    state.path = join(tmpdir(), "no-such-dir", "tuimark");
    await ext.activate(ctx());
    expect(state.clients).toHaveLength(0);
    expect(state.warnings).toHaveLength(1);
    expect(state.warnings[0]).toContain("tuimark.path");
    expect(state.warnings[0]).toContain("still work");
    await ext.deactivate();
  });

  test("a server that closes right after starting warns once", async () => {
    state.path = fakeTuimark("exit 0");
    state.closeOnStart = true;
    await ext.activate(ctx());
    expect(state.clients).toHaveLength(1);
    expect(state.warnings).toHaveLength(1);
    expect(state.warnings[0]).toContain("exited right after starting");
    // The handler is asked again (a second close): still one warning.
    expect(state.clients[0].opts.errorHandler.closed()).toEqual({ action: 1, handled: true });
    expect(state.warnings).toHaveLength(1);
    await ext.deactivate();
  });

  test("a start that rejects warns once and does not throw", async () => {
    state.path = fakeTuimark("exit 0");
    state.failStart = true;
    await ext.activate(ctx());
    expect(state.warnings).toHaveLength(1);
    expect(state.warnings[0]).toContain("did not start");
    await ext.deactivate();
  });

  test("the library's own error toast is suppressed until the server runs", async () => {
    state.path = fakeTuimark("exit 0");
    await ext.activate(ctx());
    const c = state.clients[0];
    c.error("couldn't create connection to server", new Error("x"), "force");
    expect(c.errors.at(-1)![2]).toBe(false);
    c.running = true;
    c.error("a request failed", undefined, true);
    expect(c.errors.at(-1)![2]).toBe(true);
    await ext.deactivate();
  });

  test("a healthy server starts the client for both languages, with no warning", async () => {
    state.path = fakeTuimark("exit 0");
    await ext.activate(ctx());
    expect(state.warnings).toHaveLength(0);
    const c = state.clients[0];
    expect(c.server.run).toEqual({ command: state.path, args: ["lsp"], transport: 0 });
    const langs = c.opts.documentSelector.map((s: any) => s.language);
    expect(new Set(langs)).toEqual(new Set(["tuimark", "tcss"]));
    await ext.deactivate();
  });
});
