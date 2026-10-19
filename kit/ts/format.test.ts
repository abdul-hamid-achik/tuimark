import { expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { bytes, duration, percent, relative, thousands } from "./format.ts";

// The tables in ../testdata/format.json are shared with the Go tests
// (kit/go/format_test.go): the same input must give the same string.
type Row<T> = { in: T; out: string };
const fixture = JSON.parse(readFileSync(new URL("../testdata/format.json", import.meta.url), "utf8")) as {
  bytes: Row<number>[];
  durationMs: Row<number>[];
  relativeSeconds: Row<number>[];
  percent: (Row<number> & { decimals: number })[];
  thousands: Row<number>[];
};

test("bytes", () => {
  for (const c of fixture.bytes) expect(bytes(c.in)).toBe(c.out);
});

test("duration", () => {
  for (const c of fixture.durationMs) expect(duration(c.in)).toBe(c.out);
});

test("relative", () => {
  const now = Date.UTC(2026, 9, 19, 12);
  for (const c of fixture.relativeSeconds) expect(relative(now + c.in * 1000, now)).toBe(c.out);
  expect(relative(new Date(now - 60_000), new Date(now))).toBe("1 minute ago");
});

test("percent", () => {
  for (const c of fixture.percent) expect(percent(c.in, c.decimals)).toBe(c.out);
});

test("thousands", () => {
  for (const c of fixture.thousands) expect(thousands(c.in)).toBe(c.out);
});

test("a value that is not finite is a dash, the same in Go for percent", () => {
  for (const n of [NaN, Infinity, -Infinity]) {
    expect(bytes(n)).toBe("-");
    expect(duration(n)).toBe("-");
    expect(percent(n)).toBe("-");
    expect(thousands(n)).toBe("-");
    expect(relative(n, 0)).toBe("-");
  }
});
