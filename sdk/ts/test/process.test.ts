// SPEC test 122, the parts that need a process of their own: forwarding of
// SIGTERM and SIGHUP (delayed, see FORWARD_GRACE_MS), and exitWithChild. test/parent-fixture.ts is a small
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

function runParent(scenario: object, mode: string, extraEnv: Record<string, string> = {}) {
  const { env, log } = fakeEnv(scenario);
  const proc = Bun.spawn([process.execPath, fixture], {
    env: { ...env, FAKE_BIN: fakeBin, FIXTURE_MODE: mode, ...extraEnv },
    stdin: "ignore",
    stdout: "pipe",
    stderr: "pipe",
  });
  running.push(proc);
  return { proc, log };
}

/**
 * Reads the parent's stdout up to and including the first line that is `want`.
 * Resolves with a function that reads the rest of the output, to its end.
 */
async function waitForLine(proc: ReturnType<typeof runParent>["proc"], want: string): Promise<() => Promise<string>> {
  const reader = (proc.stdout as ReadableStream<Uint8Array>).getReader();
  const decoder = new TextDecoder();
  let seen = "";
  while (!seen.split("\n").includes(want)) {
    const { value, done } = await reader.read();
    if (done) throw new Error(`parent ended before printing ${JSON.stringify(want)}; got ${JSON.stringify(seen)}`);
    seen += decoder.decode(value, { stream: true });
  }
  return async () => {
    for (;;) {
      const { value, done } = await reader.read();
      if (done) return seen;
      seen += decoder.decode(value, { stream: true });
    }
  };
}

/** The pid of the fake child, from its log. */
async function childPid(log: string): Promise<number> {
  for (let i = 0; i < 100; i++) {
    const record = readLog(log).find((r) => "pid" in r);
    if (record) return record.pid;
    await Bun.sleep(20);
  }
  throw new Error("the fake child never logged its pid");
}

const signals = (log: string) => readLog(log).filter((r) => "signal" in r);

describe("signal forwarding", () => {
  for (const [signal, code] of [
    ["SIGTERM", 143],
    ["SIGHUP", 129],
  ] as const) {
    test(`${signal} to the parent only reaches the child, after the grace; the parent exits with the child's code`, async () => {
      const { proc, log } = runParent({ hang: true }, "exitWithChild"); // the real 1500 ms
      await waitForLine(proc, "ready");
      const t0 = performance.now();
      proc.kill(signal); // the parent's pid only, as a test harness does
      expect(await proc.exited).toBe(code);
      expect(performance.now() - t0).toBeGreaterThanOrEqual(1400);
      expect(signals(log)).toEqual([{ signal }]);
    }, 15_000);
  }

  test("the forward waits: nothing reaches the child before the grace is over", async () => {
    const { proc, log } = runParent({ hang: true }, "exitWithChild", { FIXTURE_GRACE: "800" });
    await waitForLine(proc, "ready");
    proc.kill("SIGTERM");
    await Bun.sleep(300);
    expect(signals(log)).toEqual([]);
    expect(await proc.exited).toBe(143);
    expect(signals(log)).toEqual([{ signal: "SIGTERM" }]);
  }, 15_000);

  test("a second signal of the same kind during the wait is forwarded at once", async () => {
    const { proc, log } = runParent({ hang: true }, "exitWithChild", { FIXTURE_GRACE: "60000" });
    await waitForLine(proc, "ready");
    proc.kill("SIGTERM");
    await Bun.sleep(150);
    expect(signals(log)).toEqual([]);
    proc.kill("SIGTERM");
    expect(await proc.exited).toBe(143);
    expect(signals(log)).toEqual([{ signal: "SIGTERM" }]);
  }, 15_000);

  for (const [signal, code] of [
    ["SIGTERM", 143],
    ["SIGHUP", 129],
  ] as const) {
    test(`${signal} to the whole process group ends with a clean exit message, not a second signal`, async () => {
      // A group-wide signal reaches the child from the kernel and the parent from the
      // same event; under Run() a second signal is fatal. Here the child takes 150 ms to
      // end after its first, and kills itself on a second.
      const { proc, log } = runParent(
        { hang: true, signalExitDelayMs: 150, fatalSecondSignal: true },
        "report",
        { FIXTURE_GRACE: "600" },
      );
      const rest = await waitForLine(proc, "ready");
      const pid = await childPid(log);
      proc.kill(signal);
      process.kill(pid, signal);
      expect(await proc.exited).toBe(0);
      const out = await rest();
      expect(out).toContain(JSON.stringify({ reason: "signal", signal, code }));
      expect(signals(log)).toEqual([{ signal }]);
      await Bun.sleep(800); // longer than the grace: no stray forward comes after the exit
      expect(signals(log)).toEqual([{ signal }]);
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
