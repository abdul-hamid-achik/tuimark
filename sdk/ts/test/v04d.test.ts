// SPEC 0.4d tests 305 and 313 (sdk part): commit(), append(), batch entries
// with `op`, the `inline` and `screen` options, and the ready.messages check,
// against the fake child (test/fake-child.ts). The real-child parts are in
// real-child.test.ts.
import { afterEach, describe, expect, test } from "bun:test";
import { HostError, RequestError, UnsupportedMessageError } from "../src/index.ts";
import { closeAll, start } from "./helpers.ts";

afterEach(closeAll);

// What a 0.4c child (or any child older than 0.4d) lists: nothing, as its
// ready has no `messages`.
const OLD = { messages: null, hang: true };

describe("test 305: commit, append, op", () => {
  test("ready.messages is exposed as the child sent it", async () => {
    const { host } = await start({ hang: true });
    expect(host.ready.messages).toEqual(["set", "bind", "batch", "get", "reply", "append", "commit"]);
    const old = await start(OLD);
    expect(old.host.ready.messages).toBeUndefined();
  });

  test("ui.commit({lines}) and ui.append resolve with their acks", async () => {
    const { host, received } = await start({ hang: true, store: { log: "a", rows: [1] } });
    const commit = await host.commit({ lines: ["a", ["x", { text: "y", class: "c d" }]] });
    expect(commit).toMatchObject({ type: "ack", ok: true });
    const text = await host.append("log", "x");
    const row = await host.append("rows", { text: "hi" });
    expect(text).toMatchObject({ type: "ack", ok: true });
    expect(row).toMatchObject({ type: "ack", ok: true });
    expect(new Set([commit.id, text.id, row.id]).size).toBe(3);
    expect(await host.get("log")).toEqual({ found: true, value: "ax" });
    expect(await host.get("rows")).toEqual({ found: true, value: [1, { text: "hi" }] });
    expect(received().slice(0, 3)).toEqual([
      { type: "commit", lines: ["a", ["x", { text: "y", class: "c d" }]], id: commit.id },
      { type: "append", path: "log", value: "x", id: text.id },
      { type: "append", path: "rows", value: { text: "hi" }, id: row.id },
    ]);
  });

  test("ui.commit({template, with, props}) sends only the members it was given", async () => {
    const { host, received } = await start({ hang: true });
    await host.commit({ template: "card", with: "m as x", props: { tone: "warn" } });
    await host.commit({ template: "plain" });
    expect(received()).toEqual([
      { type: "commit", template: "card", with: "m as x", props: { tone: "warn" }, id: expect.any(Number) },
      { type: "commit", template: "plain", id: expect.any(Number) },
    ]);
  });

  test("an append the child refuses rejects with RequestError", async () => {
    const { host } = await start({ hang: true, store: { n: 3 } });
    const error = await host.append("n", 1).catch((e: unknown) => e);
    expect(error).toBeInstanceOf(RequestError);
    expect((error as RequestError).ack?.ok).toBe(false);
  });

  test("an undefined append value is sent as null", async () => {
    const { host, received } = await start({ hang: true, store: { rows: [] } });
    await host.append("rows", undefined);
    expect(received()[0]).toMatchObject({ type: "append", path: "rows", value: null });
  });

  test("batch entries carry their op, in the order the child reads them", async () => {
    const { host, received } = await start({ hang: true, store: { "live.text": "hello", busy: true } });
    const ack = await host.batch([
      { op: "append", path: "live.text", value: " world" },
      { path: "busy", value: false },
    ]);
    expect(ack).toMatchObject({ ok: true });
    expect(await host.get("live.text")).toEqual({ found: true, value: "hello world" });
    expect(await host.get("busy")).toEqual({ found: true, value: false });
    expect(received()[0]).toEqual({
      type: "batch",
      writes: [
        { op: "append", path: "live.text", value: " world" },
        { path: "busy", value: false },
      ],
      id: ack.id,
    });
  });
});

describe("test 305: a child without messages", () => {
  test("ui.commit rejects without sending anything", async () => {
    const { host, received } = await start(OLD);
    const error = await host.commit({ lines: ["a"] }).catch((e: unknown) => e);
    expect(error).toBeInstanceOf(UnsupportedMessageError);
    expect(error).toBeInstanceOf(HostError);
    expect((error as Error).message).toBe('the tuimark child does not accept "commit" messages');
    expect((error as UnsupportedMessageError).messageType).toBe("commit");
    // A request that follows is the first thing the child received.
    await host.get("x");
    expect(received()).toEqual([{ type: "get", path: "x", id: expect.any(Number) }]);
  });

  test("ui.append rejects without sending anything", async () => {
    const { host, received } = await start(OLD);
    const error = await host.append("log", "x").catch((e: unknown) => e);
    expect(error).toBeInstanceOf(UnsupportedMessageError);
    expect((error as Error).message).toBe('the tuimark child does not accept "append" messages');
    await host.get("x");
    expect(received().map((m) => m.type)).toEqual(["get"]);
  });

  test("a batch entry with op append rejects, where the child would have applied a set", async () => {
    const { host, received } = await start({ ...OLD, store: { log: "a" } });
    const error = await host.batch([{ op: "append", path: "log", value: "x" }]).catch((e: unknown) => e);
    expect(error).toBeInstanceOf(UnsupportedMessageError);
    expect((error as Error).message).toBe('the tuimark child does not accept "append" messages');
    expect(await host.get("log")).toEqual({ found: true, value: "a" });
    expect(received().map((m) => m.type)).toEqual(["get"]);
  });

  test("a batch without such an entry is sent unchanged, op set included", async () => {
    const { host, received } = await start({ ...OLD, store: {} });
    const ack = await host.batch([
      { path: "log", value: "x" },
      { op: "set", path: "n", value: 1 },
    ]);
    expect(ack).toMatchObject({ type: "ack", ok: true });
    expect(await host.get("log")).toEqual({ found: true, value: "x" });
    expect(await host.get("n")).toEqual({ found: true, value: 1 });
    expect(received()[0]).toEqual({
      type: "batch",
      writes: [
        { path: "log", value: "x" },
        { op: "set", path: "n", value: 1 },
      ],
      id: ack.id,
    });
  });

  test("set and get go to it as before", async () => {
    const { host } = await start(OLD);
    expect(await host.set("a", 1)).toMatchObject({ ok: true });
    expect(await host.get("a")).toEqual({ found: true, value: 1 });
  });

  test("a child that lists commit but not append rejects append and a batch op alike", async () => {
    const { host, received } = await start({
      hang: true,
      messages: ["set", "bind", "batch", "get", "reply", "commit"],
    });
    expect(await host.commit({ lines: ["a"] })).toMatchObject({ ok: true });
    await expect(host.append("a", "b")).rejects.toThrow('the tuimark child does not accept "append" messages');
    await expect(host.batch([{ op: "append", path: "a", value: "b" }])).rejects.toThrow(
      'the tuimark child does not accept "append" messages',
    );
    expect(received().map((m) => m.type)).toEqual(["commit"]);
  });

  test("a child that lists append but not commit rejects commit only", async () => {
    const { host } = await start({ hang: true, messages: ["set", "bind", "batch", "get", "reply", "append"], store: { a: "" } });
    await expect(host.commit({ lines: ["a"] })).rejects.toThrow('the tuimark child does not accept "commit" messages');
    expect(await host.append("a", "b")).toMatchObject({ ok: true });
  });
});

describe("test 305 and 313: the inline and screen options", () => {
  test("no flag by default", async () => {
    const { records } = await start({ hang: true });
    expect(records()[0]).toEqual({ argv: ["host", "app.tui"] });
  });

  test("inline: true passes --inline", async () => {
    const { records } = await start({ hang: true }, { inline: true });
    expect(records()[0]).toEqual({ argv: ["host", "app.tui", "--inline"] });
    const off = await start({ hang: true }, { inline: false });
    expect(off.records()[0]).toEqual({ argv: ["host", "app.tui"] });
  });

  test("screen: ID passes --screen ID", async () => {
    const { records } = await start({ hang: true }, { screen: "b" }, "two.tui");
    expect(records()[0]).toEqual({ argv: ["host", "two.tui", "--screen", "b"] });
  });

  test("all the options together, in the order of the CLI's flags", async () => {
    const { records } = await start(
      { hang: true },
      { theme: "light", replyTimeout: "250ms", inline: true, screen: "b" },
      "two.tui",
    );
    expect(records()[0]).toEqual({
      argv: ["host", "two.tui", "--theme", "light", "--reply-timeout", "250ms", "--inline", "--screen", "b"],
    });
  });

  test("an empty screen is passed on, for the child to refuse", async () => {
    const { records } = await start({ hang: true }, { screen: "" });
    expect(records()[0]).toEqual({ argv: ["host", "app.tui", "--screen", ""] });
  });
});
