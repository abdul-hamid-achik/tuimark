import { describe, expect, test } from "bun:test";
import { component, FakeStore } from "./fake-store.ts";
import { createToasts } from "./toasts.ts";

function setup(options: Parameters<typeof createToasts>[1] = {}) {
  const store = new FakeStore();
  let t = 1_000_000;
  const toasts = createToasts(store.host, { ...options, now: () => t });
  return { store, toasts, advance: (ms: number) => (t += ms) };
}

describe("toasts", () => {
  test("writes exactly what toast.tui reads", async () => {
    const { store, toasts } = setup();
    await toasts.push("ok", "Settings saved");
    expect(store.at("toast")).toEqual({ show: true, text: "Settings saved", is_ok: true, is_warn: false, is_danger: false });
    const source = component("toast");
    for (const p of ["toast.show", "toast.text", "toast.is_ok", "toast.is_warn", "toast.is_danger"]) {
      expect(source).toContain(p);
    }
  });

  test("shows one at a time and expires in order", async () => {
    const { store, toasts, advance } = setup();
    await toasts.push("info", "first", 2000);
    await toasts.push("danger", "second", 2000);
    expect(store.at("toast.text")).toBe("first");

    advance(1000);
    await toasts.tick();
    expect(store.at("toast.text")).toBe("first");

    advance(1000);
    await toasts.tick(); // first is up; second takes its place with a fresh clock
    expect(store.at("toast.text")).toBe("second");
    expect(store.at("toast.is_danger")).toBe(true);

    advance(1500);
    await toasts.tick();
    expect(store.at("toast.text")).toBe("second");
    advance(1000);
    await toasts.tick();
    expect(store.at("toast.show")).toBe(false);
    expect(toasts.size()).toBe(0);
  });

  test("tick writes only when something changes", async () => {
    const { store, toasts, advance } = setup();
    await toasts.tick();
    await toasts.push("info", "x", 1000);
    const before = store.batches.length;
    advance(500);
    await toasts.tick();
    expect(store.batches.length).toBe(before);
  });

  test("a full queue drops the oldest waiting toast", async () => {
    const { store, toasts, advance } = setup({ max: 3, ttl: 1000 });
    for (const s of ["a", "b", "c", "d"]) await toasts.push("info", s);
    expect(toasts.size()).toBe(3);
    const shown: unknown[] = [];
    for (let i = 0; i < 4; i++) {
      shown.push(store.at("toast.text"));
      advance(1000);
      await toasts.tick();
    }
    expect(shown).toEqual(["a", "c", "d", ""]);
  });

  test("max 1 replaces the toast on screen", async () => {
    const { store, toasts } = setup({ max: 1 });
    await toasts.push("info", "old");
    await toasts.push("warn", "new");
    expect(store.at("toast.text")).toBe("new");
    expect(store.at("toast.is_warn")).toBe(true);
  });

  test("toast_dismiss drops the one on screen and shows the next", async () => {
    const { store, toasts } = setup();
    toasts.register();
    await toasts.push("info", "one");
    await toasts.push("ok", "two");
    await store.emit("toast_dismiss", "");
    expect(store.at("toast.text")).toBe("two");
    await store.emit("toast_dismiss", "");
    expect(store.at("toast.show")).toBe(false);
    await store.emit("toast_dismiss", ""); // nothing left: harmless
  });

  test("an unknown kind is an error", async () => {
    const { toasts } = setup();
    await expect(toasts.push("error" as never, "x")).rejects.toThrow("not info, ok, warn or danger");
  });

  test("start() ticks on a timer and stop() ends it", async () => {
    const store = new FakeStore();
    const toasts = createToasts(store.host, { ttl: 1 });
    await toasts.push("info", "short-lived");
    const stop = toasts.start(5);
    await Bun.sleep(40);
    stop();
    expect(store.at("toast.show")).toBe(false);
  });
});
