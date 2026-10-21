// SPEC 0.4d tests 304, 305, and 313 (sdk part) against a REAL `tuimark host`
// child, run on a pseudo-terminal by test/pty-wrapper.py (`bun test` has no
// terminal of its own). Two children are covered, each when its binary is
// named, and skipped with the reason below when it is not (CI's sdk job has
// no Go build; the Glyphrun host_ts specs and cmd/tuimark's pty tests cover
// the same ground there):
//
//   TUIMARK_TEST_CHILD      a current tuimark binary (e.g. ../../bin/tuimark)
//   TUIMARK_TEST_OLD_CHILD  a tuimark 0.4.5 binary, whose `ready` has no
//                           `messages` (test 305's "child without messages")
//
//   TUIMARK_TEST_CHILD=../../bin/tuimark TUIMARK_TEST_OLD_CHILD=/path/to/tuimark-0.4.5 bun test test/real-child.test.ts
import { afterEach, describe, expect, test } from "bun:test";
import { chmodSync, existsSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { HostError, RequestError, UnsupportedMessageError, spawnHost, type Host } from "../src/index.ts";

const here = dirname(fileURLToPath(import.meta.url));
const python = Bun.which("python3");

const current = process.env.TUIMARK_TEST_CHILD ? resolve(process.env.TUIMARK_TEST_CHILD) : undefined;
const old = process.env.TUIMARK_TEST_OLD_CHILD ? resolve(process.env.TUIMARK_TEST_OLD_CHILD) : undefined;

const dir = mkdtempSync(join(tmpdir(), "tuimark-real-child-"));
process.on("exit", () => rmSync(dir, { recursive: true, force: true }));

const doc = join(dir, "two.tui");
writeFileSync(
  doc,
  `<tui>
  <keymap><bind keys="q" action="quit"/></keymap>
  <screen id="a"><text>SCREEN-A {log}</text></screen>
  <screen id="b"><text>SCREEN-B {log}</text></screen>
</tui>
`,
);

/** A `bin` that runs `binary` on a pty (the wrapper keeps fds 3 and 4). */
function wrapped(binary: string): string {
  const bin = join(dir, `bin-${Math.random().toString(36).slice(2)}`);
  writeFileSync(bin, `#!/bin/sh\nexec '${python}' '${join(here, "pty-wrapper.py")}' '${binary}' "$@"\n`);
  chmodSync(bin, 0o755);
  return bin;
}

const started: Host[] = [];
afterEach(async () => {
  await Promise.all(started.splice(0).map((h) => h.close()));
});

async function spawn(binary: string, options: Parameters<typeof spawnHost>[1] = {}): Promise<Host> {
  const host = await spawnHost(doc, { bin: wrapped(binary), data: { log: "a" }, replyTimeout: "1s", ...options });
  started.push(host);
  return host;
}

const skipWhy = !python
  ? "python3 is not on PATH (the pty wrapper needs it)"
  : undefined;

describe("a current child", () => {
  const why = skipWhy ?? (current && existsSync(current) ? undefined : "TUIMARK_TEST_CHILD does not name a tuimark binary");
  const t = why ? test.skip : test;
  if (why) console.log(`real-child (current): skipped, ${why}`);

  t("ready.messages lists the 0.4d types, and the protocol is still 1 (test 304)", async () => {
    const host = await spawn(current!);
    expect(host.ready.protocol).toBe(1);
    expect(host.ready.messages).toEqual(["set", "bind", "batch", "get", "reply", "append", "commit"]);
  });

  t("append and a batch entry with op append are acked and applied (tests 303 and 305)", async () => {
    const host = await spawn(current!);
    expect(await host.append("log", "x")).toMatchObject({ ok: true });
    expect(await host.get("log")).toEqual({ found: true, value: "ax" });
    await host.set("rows", []);
    expect(await host.append("rows", { text: "hi" })).toMatchObject({ ok: true });
    expect(
      await host.batch([
        { op: "append", path: "log", value: "y" },
        { path: "n", value: 1 },
      ]),
    ).toMatchObject({ ok: true });
    expect(await host.get("log")).toEqual({ found: true, value: "axy" });
    expect(await host.get("rows")).toEqual({ found: true, value: [{ text: "hi" }] });
    const error = await host.append("n", 1).catch((e: unknown) => e);
    expect(error).toBeInstanceOf(RequestError);
    expect((error as Error).message).toBe('append "n": "n" holds a number, not a string or an array');
  });

  t("commit without inline is refused by the child (test 301's last line)", async () => {
    const host = await spawn(current!);
    const error = await host.commit({ lines: ["a"] }).catch((e: unknown) => e);
    expect(error).toBeInstanceOf(RequestError);
    expect((error as Error).message).toBe("commit: not in inline mode");
  });

  t("screen: b starts on screen b, and an unknown screen rejects spawnHost (test 313)", async () => {
    const host = await spawn(current!, { screen: "b" });
    expect(await host.get("@screen")).toEqual({ found: true, value: "b" });
    const other = await spawn(current!);
    expect(await other.get("@screen")).toEqual({ found: true, value: "a" });

    const error = await spawn(current!, { screen: "nope" }).catch((e: unknown) => e);
    expect(error).toBeInstanceOf(HostError);
    expect((error as HostError).code).toBe(1);
  });
});

describe("a 0.4.5 child (no ready.messages)", () => {
  const why = skipWhy ?? (old && existsSync(old) ? undefined : "TUIMARK_TEST_OLD_CHILD does not name a tuimark 0.4.5 binary");
  const t = why ? test.skip : test;
  if (why) console.log(`real-child (0.4.5): skipped, ${why}`);

  t("commit and append reject without sending, and plain writes still go (test 305)", async () => {
    const host = await spawn(old!);
    expect(host.ready.messages).toBeUndefined();
    expect(host.ready.version).toMatch(/^0\.4\.[0-5]$/);
    await expect(host.commit({ lines: ["a"] })).rejects.toBeInstanceOf(UnsupportedMessageError);
    await expect(host.commit({ lines: ["a"] })).rejects.toThrow('the tuimark child does not accept "commit" messages');
    await expect(host.append("log", "x")).rejects.toThrow('the tuimark child does not accept "append" messages');
    // The child would have applied the entry as a set and acked it.
    await expect(host.batch([{ op: "append", path: "log", value: "x" }])).rejects.toThrow(
      'the tuimark child does not accept "append" messages',
    );
    expect(await host.get("log")).toEqual({ found: true, value: "a" });
    // An entry without an op other than "set" needs nothing.
    expect(
      await host.batch([
        { path: "log", value: "z" },
        { op: "set", path: "n", value: 1 },
      ]),
    ).toMatchObject({ ok: true });
    expect(await host.get("log")).toEqual({ found: true, value: "z" });
    expect(await host.get("n")).toEqual({ found: true, value: 1 });
  });
});
