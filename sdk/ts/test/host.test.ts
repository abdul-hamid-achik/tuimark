// SPEC test 122: @thelacanians/tuimark against a fake child (test/fake-child.ts) that
// speaks the §19.1 protocol on fd 3 and fd 4. The parts that need a process of
// their own (signals, exitWithChild) are in process.test.ts.
import { afterEach, describe, expect, test } from "bun:test";
import { existsSync, mkdtempSync, readdirSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { HostError, ProtocolVersionError, RequestError, spawnHost } from "../src/index.ts";
import { closeAll, fakeBin, fakeEnv, start } from "./helpers.ts";

afterEach(closeAll);
describe("handshake", () => {
  test("spawnHost resolves with ready, and passes the arguments", async () => {
    const dataDir = mkdtempSync(join(tmpdir(), "tuimark-host-data-"));
    const dataFile = join(dataDir, "d.json");
    writeFileSync(dataFile, '{"n":1}');
    const { host, records } = await start(
      { hang: true },
      { data: dataFile, theme: "light", replyTimeout: "250ms" },
      "view.tui",
    );
    expect(host.ready).toMatchObject({ type: "ready", protocol: 1, version: "0.4.0", cols: 80, rows: 24, theme: "dark" });
    expect(host.ready.catalog).toEqual([{ name: "quit", builtin: true, sources: [] }]);
    expect(host.pid).toBeGreaterThan(0);
    expect(records()[0]).toEqual({
      argv: ["host", "view.tui", "--data", dataFile, "--theme", "light", "--reply-timeout", "250ms"],
    });
    expect(records()).toContainEqual({ data: { n: 1 } });
  });

  test("a numeric replyTimeout is milliseconds", async () => {
    const { records } = await start({ hang: true }, { replyTimeout: 1500 });
    expect(records()[0].argv).toEqual(["host", "app.tui", "--reply-timeout", "1500ms"]);
  });

  test("data given as an object reaches the child through a temporary file that is then removed", async () => {
    const { host, records } = await start({ hang: true }, { data: { items: [1, 2], name: "é" } });
    expect(records()).toContainEqual({ data: { items: [1, 2], name: "é" } });
    const argv: string[] = records()[0].argv;
    expect(argv[2]).toBe("--data");
    expect(existsSync(argv[3]!)).toBe(false);
    expect(host.ready.protocol).toBe(1);
  });

  test("a protocol other than 1 rejects exited, stops the child, and fails requests", async () => {
    const { host, records } = await start({ protocol: 2, hang: true });
    expect(host.ready.protocol).toBe(2);
    const error = await host.exited.catch((e: unknown) => e);
    expect(error).toBeInstanceOf(ProtocolVersionError);
    expect((error as ProtocolVersionError).protocol).toBe(2);
    expect((error as HostError).code).toBe(143);
    expect(records()).toContainEqual({ signal: "SIGTERM" });
    await expect(host.set("a", 1)).rejects.toBeInstanceOf(ProtocolVersionError);
  });

  test("a child that exits before ready rejects spawnHost with its code", async () => {
    const { env } = fakeEnv({ exitBeforeReady: 1 });
    const error = await spawnHost("app.tui", { bin: fakeBin, env }).catch((e: unknown) => e);
    expect(error).toBeInstanceOf(HostError);
    expect((error as HostError).code).toBe(1);
    expect((error as HostError).message).toContain("before it sent");
  });

  test("a missing binary rejects spawnHost", async () => {
    const error = await spawnHost("app.tui", { bin: "/nonexistent/tuimark" }).catch((e: unknown) => e);
    expect(error).toBeInstanceOf(HostError);
    expect((error as HostError).message).toContain("/nonexistent/tuimark");
  });

  test("without `bin`, spawnHost runs the binary the resolver finds (TUIMARK_BINARY)", async () => {
    const saved = process.env.TUIMARK_BINARY;
    process.env.TUIMARK_BINARY = fakeBin;
    try {
      const { env } = fakeEnv({});
      const host = await spawnHost("app.tui", { env });
      await host.close();
    } finally {
      if (saved === undefined) delete process.env.TUIMARK_BINARY;
      else process.env.TUIMARK_BINARY = saved;
    }
  });

  test("without `bin`, a TUIMARK_BINARY that is not a file rejects spawnHost", async () => {
    const saved = process.env.TUIMARK_BINARY;
    process.env.TUIMARK_BINARY = "/nonexistent/tuimark";
    try {
      const error = await spawnHost("app.tui").catch((e: unknown) => e);
      expect(error).toBeInstanceOf(HostError);
      expect((error as HostError).message).toContain("cannot find the tuimark binary");
      expect((error as HostError).message).toContain("/nonexistent/tuimark");
    } finally {
      if (saved === undefined) delete process.env.TUIMARK_BINARY;
      else process.env.TUIMARK_BINARY = saved;
    }
  });

  test("messages that arrive with ready wait for the caller's handlers", async () => {
    const { host, records } = await start({ burst: true, events: [{ action: "first" }] });
    const seen: string[] = [];
    host.on("first", (ev) => {
      seen.push(ev.action);
      return "quit";
    });
    const exit = await host.exited;
    expect(seen).toEqual(["first"]);
    expect(exit).toEqual({ reason: "quit", code: 0 });
    expect(records().filter((r) => "recv" in r)).toEqual([{ recv: { type: "reply", seq: 1, quit: true } }]);
  });

  test("handlers given at spawn are in place for the first event", async () => {
    const seen: number[] = [];
    const { host } = await start(
      { burst: true, events: [{ action: "go" }] },
      { handlers: { go: (ev) => void seen.push(ev.seq) } },
    );
    await host.exited;
    expect(seen).toEqual([1]);
  });
});

describe("acks", () => {
  test("set and batch resolve with the ack, each with a fresh id", async () => {
    const { host, received } = await start({ hang: true });
    const a = await host.set("x", { deep: [1] });
    const b = await host.batch([
      { path: "y", value: 2 },
      { path: "z", value: null },
    ]);
    const c = await host.set("w", 3);
    expect(a).toMatchObject({ type: "ack", ok: true });
    expect(b).toMatchObject({ type: "ack", ok: true });
    const ids = [a.id, b.id, c.id];
    expect(new Set(ids).size).toBe(3);
    expect(received()).toEqual([
      { type: "set", path: "x", value: { deep: [1] }, id: a.id },
      { type: "batch", writes: [{ path: "y", value: 2 }, { path: "z", value: null }], id: b.id },
      { type: "set", path: "w", value: 3, id: c.id },
    ]);
  });

  test("an undefined value is sent as null", async () => {
    const { host, received } = await start({ hang: true });
    await host.set("x", undefined);
    expect(received()[0]).toMatchObject({ path: "x", value: null });
  });

  test("get resolves with found and value", async () => {
    const { host } = await start({ hang: true, store: { here: { a: 1 }, zero: 0 } });
    expect(await host.get("here")).toEqual({ found: true, value: { a: 1 } });
    expect(await host.get("zero")).toEqual({ found: true, value: 0 });
    expect(await host.get("gone")).toEqual({ found: false });
    await host.set("fresh", "v");
    expect(await host.get("fresh")).toEqual({ found: true, value: "v" });
  });

  test("an ack with ok:false rejects with RequestError carrying the ack", async () => {
    const { host } = await start({ hang: true });
    const error = await host.set("bad", 1).catch((e: unknown) => e);
    expect(error).toBeInstanceOf(RequestError);
    expect((error as RequestError).message).toBe("boom: refused");
    expect((error as RequestError).ack).toMatchObject({ ok: false, error: "boom: refused" });
    await expect(host.batch([{ path: "bad", value: 1 }])).rejects.toBeInstanceOf(RequestError);
  });

  test("an error that echoes the id rejects that request only", async () => {
    const { host } = await start({ hang: true });
    const failed = host.set("malformed", 1);
    const fine = host.set("ok", 1);
    await expect(failed).rejects.toThrow("malformed message");
    expect((await fine).ok).toBe(true);
  });

  test("a line of 64 MiB or more is refused before it is sent", async () => {
    const { host, received } = await start({ hang: true });
    const error = await host.set("big", "x".repeat(64 * 1024 * 1024)).catch((e: unknown) => e);
    expect(error).toBeInstanceOf(RangeError);
    await host.set("small", 1); // the session is fine, and nothing of the big line was sent
    expect(received().map((m) => m.path)).toEqual(["small"]);
  }, 20_000);

  test("requests fail once the child has exited", async () => {
    const { host } = await start({});
    await host.exited;
    await expect(host.set("a", 1)).rejects.toBeInstanceOf(HostError);
  });
});

describe("large messages", () => {
  // On Bun 1.4.2 a write of 8-16 KB or more to the raw fd 3 threw inside
  // fs.createWriteStream, and nothing was ever acked again.
  for (const [label, size] of [
    ["16 KB", 16 * 1024],
    ["1 MB", 1024 * 1024],
    ["8 MB", 8 * 1024 * 1024],
  ] as const) {
    test(`a ${label} set is acked, arrives intact, and the session goes on`, async () => {
      const { host, received } = await start({ hang: true });
      const value = "é".repeat(size / 2) + "x".repeat(size % 2);
      const ack = await host.set("big", value);
      expect(ack).toMatchObject({ type: "ack", ok: true });
      expect(received()[0]).toMatchObject({ type: "set", path: "big", value: `<${value.length} chars>` });
      expect(await host.get("big")).toEqual({ found: true, value });
      expect((await host.set("after", 1)).ok).toBe(true);
    }, 30_000);
  }

  test("a large batch and a large get are acked", async () => {
    const { host } = await start({ hang: true });
    const a = "a".repeat(300_000);
    const b = "b".repeat(300_000);
    const ack = await host.batch([
      { path: "a", value: a },
      { path: "b", value: b },
    ]);
    expect(ack.ok).toBe(true);
    expect(await host.get("b")).toEqual({ found: true, value: b });
  }, 30_000);

  test("large and small messages sent together are applied in the order they were sent", async () => {
    const { host, received } = await start({ hang: true });
    const big = (c: string) => c.repeat(2 * 1024 * 1024);
    const sent: Array<Promise<unknown>> = [
      host.set("k", "first"),
      host.set("k", big("1")),
      host.set("small", 1),
      host.batch([{ path: "k", value: big("2") }]),
      host.set("k", "last"),
      host.get("k"),
      host.set("k", big("3")),
      host.get("k"),
    ];
    const results = await Promise.all(sent);
    expect((results[5] as { value: unknown }).value).toBe("last");
    expect((results[7] as { value: unknown }).value).toBe(big("3"));
    expect(received().map((m) => (m.type === "batch" ? "batch" : `${m.type}:${m.path}`))).toEqual([
      "set:k",
      "set:k",
      "set:small",
      "batch",
      "set:k",
      "get:k",
      "set:k",
      "get:k",
    ]);
    expect(received().map((m) => m.id)).toEqual([1, 2, 3, 4, 5, 6, 7, 8]);
  }, 30_000);

  test("a reply written after a large set still reaches the child", async () => {
    const { host } = await start({ events: [{ action: "go" }, { action: "again" }], hang: true });
    const seen: string[] = [];
    host.on("go", async () => {
      await host.set("big", "x".repeat(1024 * 1024));
      seen.push("go");
    });
    const second = new Promise<void>((resolve) => host.on("again", () => void (seen.push("again"), resolve())));
    await second;
    expect(seen).toEqual(["go", "again"]);
  }, 30_000);
});

describe("descriptors", () => {
  const openFds = () => readdirSync(process.platform === "linux" ? "/proc/self/fd" : "/dev/fd").length;

  test("twenty sequential sessions leave no descriptor open", async () => {
    const session = async () => {
      const { host } = await start({ events: [{ action: "tick" }] });
      host.on("tick", () => {});
      await host.exited;
    };
    await session(); // warm-up: whatever the runtime opens once
    const before = openFds();
    for (let i = 0; i < 20; i++) await session();
    expect(openFds() - before).toBeLessThanOrEqual(2);
  }, 60_000);

  test("sessions that fail leave no descriptor open either", async () => {
    const failing = async () => {
      const { host } = await start({ noExitMessage: true, code: 3 });
      await host.exited.catch(() => {});
    };
    await failing();
    const before = openFds();
    for (let i = 0; i < 10; i++) await failing();
    expect(openFds() - before).toBeLessThanOrEqual(2);
  }, 60_000);
});

describe("replies", () => {
  test("a handler that resolves replies plainly", async () => {
    const { host, received } = await start({ events: [{ action: "a" }, { action: "b" }] });
    const calls: unknown[] = [];
    host.on("a", async (ev) => {
      calls.push(ev);
      return 42; // anything but "quit"
    });
    host.on("b", () => {});
    await host.exited;
    expect(calls).toEqual([{ seq: 1, action: "a", source: "", keys: {}, value: null }]);
    expect(received()).toEqual([
      { type: "reply", seq: 1 },
      { type: "reply", seq: 2 },
    ]);
  });

  test("the event carries source, keys, and value", async () => {
    const { host } = await start({
      events: [{ action: "open", source: "list", keys: { item: "a1" }, value: { n: 1 } }],
    });
    let got: unknown;
    host.on("open", (ev) => void (got = ev));
    await host.exited;
    expect(got).toEqual({ seq: 1, action: "open", source: "list", keys: { item: "a1" }, value: { n: 1 } });
  });

  test('a handler that resolves to "quit" replies quit:true', async () => {
    const { host, received } = await start({ events: [{ action: "bye" }, { action: "never" }] });
    host.on("bye", async () => "quit");
    const exit = await host.exited;
    expect(exit).toEqual({ reason: "quit", code: 0 });
    expect(received()).toEqual([{ type: "reply", seq: 1, quit: true }]);
  });

  test("a throw replies with its message as error, which ends the session", async () => {
    const { host, received } = await start({ events: [{ action: "boom" }, { action: "never" }] });
    host.on("boom", () => {
      throw new Error("handler failed");
    });
    const exit = await host.exited;
    expect(exit).toEqual({ reason: "error", error: "handler failed", code: 1 });
    expect(received()).toEqual([{ type: "reply", seq: 1, error: "handler failed" }]);
  });

  test("a throw with an empty message still replies with error:\"\"", async () => {
    const { host, received } = await start({ events: [{ action: "boom" }] });
    host.on("boom", async () => {
      throw new Error("");
    });
    const exit = await host.exited;
    expect(exit).toMatchObject({ reason: "error", error: "" });
    expect(received()).toEqual([{ type: "reply", seq: 1, error: "" }]);
  });

  test("a thrown non-Error is reported by its string form", async () => {
    const { host, received } = await start({ events: [{ action: "boom" }] });
    host.on("boom", () => {
      throw "plain string";
    });
    await host.exited;
    expect(received()).toEqual([{ type: "reply", seq: 1, error: "plain string" }]);
  });

  test("onError turns a throw into a plain reply, and the session goes on", async () => {
    const caught: Array<{ seq: number; message: string }> = [];
    const { host, received } = await start(
      { events: [{ action: "boom" }, { action: "fine" }] },
      {
        onError: (ev, err) => {
          caught.push({ seq: ev.seq, message: (err as Error).message });
        },
      },
    );
    host.on("boom", () => {
      throw new Error("recoverable");
    });
    host.on("fine", () => {});
    const exit = await host.exited;
    expect(caught).toEqual([{ seq: 1, message: "recoverable" }]);
    expect(exit.reason).toBe("quit");
    expect(received()).toEqual([
      { type: "reply", seq: 1 },
      { type: "reply", seq: 2 },
    ]);
  });

  test("an onError that throws in turn replies with that error", async () => {
    const { host, received } = await start(
      { events: [{ action: "boom" }] },
      {
        onError: async () => {
          throw new Error("onError failed too");
        },
      },
    );
    host.on("boom", () => {
      throw new Error("first");
    });
    const exit = await host.exited;
    expect(exit).toMatchObject({ reason: "error", error: "onError failed too" });
    expect(received()).toEqual([{ type: "reply", seq: 1, error: "onError failed too" }]);
  });

  test("an event with no handler is answered at once, plainly", async () => {
    const { host, received } = await start({ events: [{ action: "nobody" }, { action: "nobody" }] });
    const exit = await host.exited;
    expect(exit.reason).toBe("quit");
    expect(received()).toEqual([
      { type: "reply", seq: 1 },
      { type: "reply", seq: 2 },
    ]);
  });

  test("writes made before the handler returns reach the child before the reply", async () => {
    const { host, received } = await start({ events: [{ action: "inc" }] });
    host.on("inc", () => {
      void host.set("count", 1); // not awaited
      void host.set("last", true);
    });
    await host.exited;
    expect(received().map((m) => m.type)).toEqual(["set", "set", "reply"]);
  });

  test("a handler can read and write state before it replies", async () => {
    const { host, received } = await start({ events: [{ action: "load" }], store: { n: 5 } });
    host.on("load", async () => {
      const { value } = await host.get("n");
      await host.set("m", (value as number) + 1);
    });
    await host.exited;
    expect(received().map((m) => m.type)).toEqual(["get", "set", "reply"]);
    expect(received()[1]).toMatchObject({ path: "m", value: 6 });
  });

  test("an error with no pending request goes to onProtocolError; noise is ignored", async () => {
    const seen: unknown[] = [];
    const { host } = await start(
      {
        events: [{ action: "x" }],
        noise: [
          "this is not json",
          "[1,2]",
          { type: "future-message", n: 1 },
          { type: "error", error: "reply timeout", seq: 4 },
          { type: "error", error: "no id", id: "zz", seq: 5 },
        ],
      },
      { onProtocolError: (m) => void seen.push(m) },
    );
    host.on("x", () => {});
    expect(await host.exited).toEqual({ reason: "quit", code: 0 });
    expect(seen).toEqual([
      { error: "reply timeout", seq: 4 },
      { error: "no id", id: "zz", seq: 5 },
    ]);
  });
});

describe("exited", () => {
  test("resolves with reason and the child's exit code", async () => {
    const { host } = await start({ code: 0 });
    expect(await host.exited).toEqual({ reason: "quit", code: 0 });
  });

  test("carries the reason, error text, and a nonzero code", async () => {
    const { host } = await start({ reason: "error", code: 1 });
    expect(await host.exited).toEqual({ reason: "error", code: 1 });
  });

  test("passes any exit reason through", async () => {
    const { host } = await start({ reason: "eof" });
    expect(await host.exited).toEqual({ reason: "eof", code: 0 });
  });

  test("a child that exits without an exit message rejects exited", async () => {
    const { host } = await start({ noExitMessage: true, code: 3 });
    const error = await host.exited.catch((e: unknown) => e);
    expect(error).toBeInstanceOf(HostError);
    expect(error).not.toBeInstanceOf(ProtocolVersionError);
    expect((error as HostError).code).toBe(3);
    expect((error as HostError).message).toContain('without an "exit" message');
  });

  test("a request pending when the child dies without an exit message rejects too", async () => {
    const { host } = await start({ hang: true, dieOn: "set" });
    const error = await host.set("a", 1).catch((e: unknown) => e);
    expect(error).toBeInstanceOf(HostError);
    expect((error as HostError).code).toBe(7);
    await expect(host.exited).rejects.toBeInstanceOf(HostError);
  });
});

describe("close", () => {
  test("sends SIGTERM, and the child reports the signal", async () => {
    const { host, records } = await start({ hang: true });
    const before = process.listenerCount("SIGTERM");
    expect(before).toBeGreaterThan(0);
    await host.close();
    expect(await host.exited).toEqual({ reason: "signal", signal: "SIGTERM", code: 143 });
    expect(records()).toContainEqual({ signal: "SIGTERM" });
    expect(process.listenerCount("SIGTERM")).toBe(before - 1);
    await host.close(); // idempotent
  });

  test("sends SIGKILL after 2 seconds to a child that ignores SIGTERM", async () => {
    const { host, records } = await start({ hang: true, ignoreTerm: true });
    const t0 = performance.now();
    await host.close();
    const elapsed = performance.now() - t0;
    expect(elapsed).toBeGreaterThanOrEqual(1900);
    expect(elapsed).toBeLessThan(5000);
    expect(records()).toContainEqual({ signal: "SIGTERM" });
    const error = await host.exited.catch((e: unknown) => e);
    expect(error).toBeInstanceOf(HostError); // SIGKILL: no exit message
    expect((error as HostError).code).toBe(137);
  }, 10_000);
});
