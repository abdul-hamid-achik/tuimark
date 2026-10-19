import { describe, expect, test } from "bun:test";
import { createDialogs } from "./dialog.ts";
import { component, FakeStore } from "./fake-store.ts";

describe("dialogs", () => {
  test("confirm opens the instance with what confirm.tui reads", async () => {
    const store = new FakeStore();
    const dialogs = createDialogs(store.host);
    await dialogs.confirm("confirm", "Delete project", "This permanently deletes orbit-api.", () => {});
    expect(store.at("confirm")).toEqual({
      open: true, title: "Delete project", message: "This permanently deletes orbit-api.", result: "none",
    });
    const source = component("confirm");
    for (const p of ["confirm.open", "confirm.title", "confirm.message", "confirm.result"]) expect(source).toContain(p);
  });

  test("dialog_confirm runs onConfirm; dialog_cancel and esc do not", async () => {
    const store = new FakeStore();
    const dialogs = createDialogs(store.host);
    dialogs.register();
    let deleted = 0;
    const ask = () => dialogs.confirm("confirm", "Delete", "sure?", () => void deleted++);

    await ask();
    await store.emit("dialog_confirm", "confirm__yes");
    expect(deleted).toBe(1);
    expect([store.at("confirm.open"), store.at("confirm.result")]).toEqual([false, "confirmed"]);

    await ask();
    await store.emit("dialog_cancel", "confirm__no");
    expect(deleted).toBe(1);
    expect([store.at("confirm.open"), store.at("confirm.result")]).toEqual([false, "cancelled"]);
  });

  test("answers route by instance", async () => {
    const store = new FakeStore();
    const dialogs = createDialogs(store.host);
    const got: string[] = [];
    await dialogs.open("one", { title: "1", body: "", onConfirm: () => got.push("one yes"), onCancel: () => got.push("one no") });
    await dialogs.confirm("two", "2", "", () => got.push("two yes"));
    await dialogs.cancelled({ seq: 1, action: "dialog_cancel", source: "one__no", keys: {}, value: null });
    await dialogs.confirmed({ seq: 2, action: "dialog_confirm", source: "two__yes", keys: {}, value: null });
    expect(got).toEqual(["one no", "two yes"]);
    expect([store.at("one.result"), store.at("two.result")]).toEqual(["cancelled", "confirmed"]);
    await dialogs.confirmed({ seq: 3, action: "dialog_confirm", source: "two__yes", keys: {}, value: null });
    expect(got.length).toBe(2); // an answered dialog runs nothing twice
  });

  test("a callback may open the dialog again", async () => {
    const store = new FakeStore();
    const dialogs = createDialogs(store.host);
    await dialogs.confirm("c", "first", "", () => dialogs.confirm("c", "second", ""));
    await dialogs.confirmed({ seq: 1, action: "dialog_confirm", source: "c__yes", keys: {}, value: null });
    expect([store.at("c.open"), store.at("c.title")]).toEqual([true, "second"]);
  });
});
