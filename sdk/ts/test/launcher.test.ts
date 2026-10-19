// The `tuimark` launcher (bin/tuimark.js) against stand-in binaries: arguments,
// exit code, signals, and fd 3/4 reach the program it runs. The launcher runs
// under Bun here (it imports ../src/binary.ts when dist/ is not built) and,
// when `node` is on PATH and `bun run build` has made dist/, under Node too.

import { afterAll, beforeAll, describe, expect, test } from "bun:test";
import { spawn } from "node:child_process";
import { chmodSync, existsSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const launcher = join(here, "..", "bin", "tuimark.js");
const dist = join(here, "..", "dist", "binary.js");

const posix = process.platform !== "win32";
const runtimes: [string, string][] = [["bun", process.execPath]];
if (existsSync(dist) && Bun.which("node")) runtimes.push(["node", Bun.which("node") as string]);

let dir: string;
beforeAll(() => {
  dir = mkdtempSync(join(tmpdir(), "tuimark-launcher-test-"));
});
afterAll(() => rmSync(dir, { recursive: true, force: true }));

function script(name: string, body: string): string {
  const path = join(dir, name);
  writeFileSync(path, `#!/bin/sh\n${body}\n`);
  chmodSync(path, 0o755);
  return path;
}

interface Result {
  code: number | null;
  signal: NodeJS.Signals | null;
  stdout: string;
  stderr: string;
  fd4: string;
}

function run(
  runtime: string,
  args: string[],
  env: Record<string, string>,
  options: { extraFds?: boolean; onStdout?: (child: ReturnType<typeof spawn>, text: string) => void } = {},
): Promise<Result> {
  return new Promise((resolve, reject) => {
    const child = spawn(runtime, [launcher, ...args], {
      env: { ...process.env, ...env },
      stdio: options.extraFds ? ["ignore", "pipe", "pipe", "pipe", "pipe"] : ["ignore", "pipe", "pipe"],
    });
    let stdout = "";
    let stderr = "";
    let fd4 = "";
    child.stdout?.on("data", (d) => {
      stdout += d;
      options.onStdout?.(child, stdout);
    });
    child.stderr?.on("data", (d) => (stderr += d));
    (child.stdio[4] as NodeJS.ReadableStream | null | undefined)?.on("data", (d) => (fd4 += d));
    child.on("error", reject);
    child.on("close", (code, signal) => resolve({ code, signal, stdout, stderr, fd4 }));
  });
}

describe.skipIf(!posix).each(runtimes)("launcher under %s", (_name, runtime) => {
  test("passes arguments through and exits with the program's code", async () => {
    const bin = script("args-" + _name, 'printf "%s|" "$@"; exit 7');
    const r = await run(runtime, ["validate", "my app.tui", "--data", "x.json"], { TUIMARK_BINARY: bin });
    expect(r.stdout).toBe("validate|my app.tui|--data|x.json|");
    expect(r.code).toBe(7);
  });

  test("forwards stderr and a zero exit", async () => {
    const bin = script("err-" + _name, "echo oops >&2");
    const r = await run(runtime, [], { TUIMARK_BINARY: bin });
    expect(r.stderr).toBe("oops\n");
    expect(r.code).toBe(0);
  });

  test("a missing TUIMARK_BINARY is one clear line and exit 1", async () => {
    const r = await run(runtime, ["version"], { TUIMARK_BINARY: join(dir, "does-not-exist") });
    expect(r.code).toBe(1);
    expect(r.stderr).toStartWith("tuimark: TUIMARK_BINARY is set to ");
  });

  test("an unverifiable download is never run", async () => {
    const home = mkdtempSync(join(dir, "home-"));
    const ran = join(home, "ran");
    const server = Bun.serve({
      port: 0,
      fetch(request) {
        const path = new URL(request.url).pathname;
        if (path.endsWith("checksums.txt")) return new Response(`${"0".repeat(64)}  anything\n`);
        return new Response(`#!/bin/sh\ntouch ${ran}\n`);
      },
    });
    try {
      const r = await run(runtime, ["version"], {
        TUIMARK_BINARY: "",
        TUIMARK_RELEASE_URL: `http://127.0.0.1:${server.port}/r`,
        HOME: home,
        XDG_CACHE_HOME: join(home, "cache"),
      });
      expect(r.code).toBe(1);
      expect(r.stderr).toContain("tuimark:");
      expect(existsSync(ran)).toBe(false);
    } finally {
      void server.stop(true);
    }
  });

  test("dies from the signal that ended the program", async () => {
    const bin = script("selfkill-" + _name, "kill -TERM $$; sleep 5");
    const r = await run(runtime, [], { TUIMARK_BINARY: bin });
    expect(r.signal).toBe("SIGTERM");
  });

  test("forwards SIGTERM to the program", async () => {
    const bin = script(
      "trap-" + _name,
      "trap 'echo got-term; exit 9' TERM\necho ready\nwhile :; do sleep 0.05; done",
    );
    let sent = false;
    const r = await run(runtime, [], { TUIMARK_BINARY: bin }, {
      onStdout(child, text) {
        if (!sent && text.includes("ready")) {
          sent = true;
          child.kill("SIGTERM");
        }
      },
    });
    expect(r.stdout).toContain("got-term");
    expect(r.code).toBe(9);
  });

  test("`host` passes fd 3 and fd 4 to the program", async () => {
    const bin = script("fds-" + _name, 'read line <&3; echo "fd4:$line" >&4');
    const child = spawn(runtime, [launcher, "host", "app.tui"], {
      env: { ...process.env, TUIMARK_BINARY: bin },
      stdio: ["ignore", "ignore", "pipe", "pipe", "pipe"],
    });
    let fd4 = "";
    (child.stdio[4] as NodeJS.ReadableStream).on("data", (d) => (fd4 += d));
    (child.stdio[3] as NodeJS.WritableStream).write("hello\n");
    const code = await new Promise<number | null>((resolve) => child.on("close", resolve));
    expect(code).toBe(0);
    expect(fd4).toBe("fd4:hello\n");
  });

  test("other commands do not pass fd 3 and fd 4", async () => {
    const bin = script("nofds-" + _name, "echo x >&4 2>/dev/null && echo leaked || echo closed");
    const r = await run(runtime, ["validate"], { TUIMARK_BINARY: bin }, { extraFds: true });
    expect(r.stdout.trim()).toBe("closed");
    expect(r.fd4).toBe("");
  });

  test("`host` with no parent descriptors still runs the program", async () => {
    // Node and Bun hold descriptors of their own at 3 and 4: passing them on may be
    // refused, and the program must run anyway.
    const bin = script("hostnofds-" + _name, "echo hosted");
    const r = await run(runtime, ["host", "app.tui"], { TUIMARK_BINARY: bin });
    expect(r.stderr).toBe("");
    expect(r.stdout.trim()).toBe("hosted");
    expect(r.code).toBe(0);
  });
});
