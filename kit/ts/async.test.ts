import { describe, expect, test } from "bun:test";
import { createLoader } from "./async.ts";
import { FakeStore } from "./fake-store.ts";

describe("loader", () => {
  test("writes loading, then data", async () => {
    const store = new FakeStore();
    const loader = createLoader(store.host);
    let release!: () => void;
    const gate = new Promise<void>((r) => (release = r));
    const task = await loader.load("repos", async () => {
      await gate;
      return [{ name: "tuimark" }];
    });
    expect(store.at("repos.loading")).toBe(true);
    expect(store.at("repos.error")).toBe("");
    expect(store.at("repos.data")).toBeNull();
    release();
    await task.done;
    expect(store.at("repos.loading")).toBe(false);
    expect(store.at("repos.data")).toEqual([{ name: "tuimark" }]);
  });

  test("an error keeps the old data", async () => {
    const store = new FakeStore();
    const loader = createLoader(store.host);
    await (await loader.load("x", async () => "v1")).done;
    await (await loader.load("x", async () => { throw new Error("503 from upstream"); })).done;
    expect(store.at("x.error")).toBe("503 from upstream");
    expect(store.at("x.loading")).toBe(false);
    expect(store.at("x.data")).toBe("v1");
    await (await loader.load("x", async () => "v3")).done;
    expect(store.at("x.error")).toBe("");
    expect(store.at("x.data")).toBe("v3");
  });

  test("a later load drops the stale result", async () => {
    const store = new FakeStore();
    const loader = createLoader(store.host);
    const slow = await loader.load("q", (signal) => new Promise((resolve) => signal.addEventListener("abort", () => resolve("stale"))));
    const fast = await loader.load("q", async () => "fresh");
    await Promise.all([slow.done, fast.done]);
    expect(store.at("q.data")).toBe("fresh");
    expect(store.at("q.loading")).toBe(false);
  });

  test("cancel drops the result", async () => {
    const store = new FakeStore();
    const loader = createLoader(store.host);
    const task = await loader.load("c", (signal) => new Promise((resolve) => signal.addEventListener("abort", () => resolve("late"))));
    task.cancel();
    await task.done;
    expect(store.at("c.data")).toBeNull();
    expect(store.at("c.loading")).toBe(false);
  });

  test("different paths do not interfere", async () => {
    const store = new FakeStore();
    const loader = createLoader(store.host);
    const [a, b] = await Promise.all([loader.load("a", async () => 1), loader.load("b", async () => 2)]);
    await Promise.all([a.done, b.done]);
    expect([store.at("a.data"), store.at("b.data")]).toEqual([1, 2]);
  });
});
