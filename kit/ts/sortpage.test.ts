import { describe, expect, test } from "bun:test";
import { FakeStore } from "./fake-store.ts";
import { createSortPage, pageLabel, sortRows } from "./sortpage.ts";

const services = [
  { id: "api", name: "api-gateway", rps: 1204 },
  { id: "auth", name: "auth", rps: 310 },
  { id: "billing", name: "billing", rps: 27 },
  { id: "search", name: "search", rps: 88 },
  { id: "mailer", name: "mailer", rps: 0 },
];
const columns = [
  { key: "name", title: "SERVICE" },
  { key: "rps", title: "REQ/S" },
];
const ids = (rows: unknown) => (rows as { id: string }[]).map((r) => r.id);

async function setup(size = 50) {
  const store = new FakeStore({ all: services });
  const sp = createSortPage(store.host, { source: "all", out: "services", meta: "svc_meta", state: "svc_state", columns, size });
  await sp.recompute();
  return { store, sp };
}

describe("sortpage", () => {
  test("cycling a column sorts ascending, descending, then back", async () => {
    const { store, sp } = await setup();
    expect(ids(store.at("services"))).toEqual(["api", "auth", "billing", "search", "mailer"]);
    await sp.cycle("name");
    expect(ids(store.at("services"))).toEqual(["api", "auth", "billing", "mailer", "search"]);
    await sp.cycle("name");
    expect(ids(store.at("services"))).toEqual(["search", "mailer", "billing", "auth", "api"]);
    await sp.cycle("name");
    expect(ids(store.at("services"))).toEqual(["api", "auth", "billing", "search", "mailer"]);
  });

  test("numbers sort numerically", async () => {
    const { store, sp } = await setup();
    await sp.cycle("rps");
    expect(ids(store.at("services"))).toEqual(["mailer", "billing", "search", "auth", "api"]);
  });

  test("titles carry the arrow of the sorted column", async () => {
    const { store, sp } = await setup();
    expect(store.at("svc_meta.titles.name")).toBe("SERVICE");
    await sp.cycle("name");
    expect(store.at("svc_meta.titles.name")).toBe("SERVICE ▲");
    expect(store.at("svc_meta.titles.rps")).toBe("REQ/S");
    await sp.cycle("name");
    expect(store.at("svc_meta.titles.name")).toBe("SERVICE ▼");
    await sp.cycle("rps");
    expect([store.at("svc_meta.titles.name"), store.at("svc_meta.titles.rps")]).toEqual(["SERVICE", "REQ/S ▲"]);
    await expect(sp.cycle("nope")).rejects.toThrow("not a column");
  });

  test("pages, label and clamping", async () => {
    const { store, sp } = await setup(2);
    expect(ids(store.at("services"))).toEqual(["api", "auth"]);
    expect(store.at("svc_meta.label")).toBe("1–2 of 5");
    await sp.nextPage();
    expect(ids(store.at("services"))).toEqual(["billing", "search"]);
    await sp.nextPage();
    await sp.nextPage(); // stops at the last page
    expect(ids(store.at("services"))).toEqual(["mailer"]);
    expect(store.at("svc_meta.label")).toBe("5–5 of 5");
    expect([store.at("svc_meta.has_next"), store.at("svc_meta.has_prev")]).toEqual([false, true]);
    await sp.cycle("name"); // sorting goes back to page 1
    expect(store.at("svc_meta.page")).toBe(1);
    await sp.setSize(10);
    expect(ids(store.at("services")).length).toBe(5);
    await sp.setPage(9); // clamped
    expect(store.at("svc_meta.page")).toBe(1);
    await sp.prevPage();
    expect(store.at("svc_meta.page")).toBe(1);
  });

  test("sortRows keeps missing values last and leaves the input alone", () => {
    const rows = [{ id: "a", v: 10 }, { id: "b", v: 9 }, { id: "c" }, { id: "d", v: 100 }];
    expect(ids(sortRows(rows, "v", false))).toEqual(["b", "a", "d", "c"]);
    expect(ids(sortRows(rows, "v", true))).toEqual(["d", "a", "b", "c"]);
    expect(ids(rows)).toEqual(["a", "b", "c", "d"]);
    const text = [{ id: "1", s: "banana" }, { id: "2", s: "Apple" }, { id: "3", s: "cherry" }];
    expect(ids(sortRows(text, "s", false))).toEqual(["2", "1", "3"]);
    expect(ids(sortRows(text, "", false))).toEqual(["1", "2", "3"]);
  });

  test("pageLabel", () => {
    expect(pageLabel(1, 50, 340)).toBe("1–50 of 340");
    expect(pageLabel(7, 50, 340)).toBe("301–340 of 340");
    expect(pageLabel(1, 50, 0)).toBe("0 of 0");
    expect(pageLabel(1, 50, 3)).toBe("1–3 of 3");
  });
});
