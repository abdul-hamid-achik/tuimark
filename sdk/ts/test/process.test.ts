// SPEC test 122, the parts that need a process of their own: forwarding of
// SIGTERM and SIGHUP, and exitWithChild. test/parent-fixture.ts is a small
// parent built on the package; the child is the fake of test/fake-child.ts.
import { afterEach, describe, expect, test } from "bun:test";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { fakeBin, fakeEnv, readLog } from "./helpers.ts";

const fixture = join(dirname(fileURLToPath(import.meta.url)), "parent-fixture.ts");
const running: Array<ReturnType<typeof Bun.spawn>> = [];

afterEach(() => {
  for (const p of running.splice(0)) p.kill("SIGKILL");
});

function runParent(scenario: object, mode: string) {
  const { env, log } = fakeEnv(scenario);
  const proc = Bun.spawn([process.execPath, fixture], {
    env: { ...env, FAKE_BIN: fakeBin, FIXTURE_MODE: mode },
    stdin: "ignore",
    stdout: "pipe",
    stderr: "pipe",
  });
  running.push(proc);
  return { proc, log };
}

/** Reads the parent's stdout up to and including the first line that is `want`. */
async function waitForLine(proc: ReturnType<typeof runParent>["proc"], want: string): Promise<void> {
  const reader = (proc.stdout as ReadableStream<Uint8Array>).getReader();
  const decoder = new TextDecoder();
  let seen = "";
  while (!seen.split("\n").includes(want)) {
    const { value, done } = await reader.read();
    if (done) throw new Error(`parent ended before printing ${JSON.stringify(want)}; got ${JSON.stringify(seen)}`);
    seen += decoder.decode(value, { stream: true });
  }
  reader.releaseLock();
}

describe("signal forwarding", () => {
  for (const [signal, code] of [
    ["SIGTERM", 143],
    ["SIGHUP", 129],
  ] as const) {
    test(`${signal} to the parent reaches the child; the parent exits with the child's code`, async () => {
      const { proc, log } = runParent({ hang: true }, "exitWithChild");
      await waitForLine(proc, "ready");
      proc.kill(signal); // the parent's pid only, as a test harness does
      expect(await proc.exited).toBe(code);
      expect(readLog(log)).toContainEqual({ signal });
    }, 15_000);
  }
});

describe("exitWithChild", () => {
  test("exits with the child's code: 0 for quit", async () => {
    const { proc } = runParent({}, "exitWithChild");
    expect(await proc.exited).toBe(0);
  });

  test("exits with the child's nonzero code", async () => {
    const { proc } = runParent({ reason: "error", code: 5 }, "exitWithChild");
    expect(await proc.exited).toBe(5);
  });

  test("a child that exits without an exit message fails the parent and says why", async () => {
    const { proc } = runParent({ noExitMessage: true, code: 3 }, "exitWithChild");
    expect(await proc.exited).toBe(3);
    expect(await new Response(proc.stderr as ReadableStream).text()).toContain('without an "exit" message');
  });

  test("a child that dies without an exit message and with code 0 still fails the parent", async () => {
    const { proc } = runParent({ noExitMessage: true, code: 0 }, "exitWithChild");
    // noExitMessage with code 0 exits 0 (`scenario.code ?? 3`); the parent must not report success.
    expect(await proc.exited).toBe(1);
  });

  test("is off by default: the parent keeps its process, and its signal listeners are gone", async () => {
    const { proc } = runParent({ code: 4, reason: "error" }, "report");
    expect(await proc.exited).toBe(0);
    const out = await new Response(proc.stdout as ReadableStream).text();
    expect(out).toContain('{"reason":"error","code":4}');
    expect(out).toContain("listeners 0 0");
  });
});
