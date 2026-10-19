import { expect, test } from "bun:test";
import { bytes, duration, percent, relative, thousands } from "./format.ts";

test("bytes", () => {
  const cases: [number, string][] = [
    [0, "0 B"], [999, "999 B"], [1023, "1023 B"], [1024, "1.0 KiB"], [1536, "1.5 KiB"],
    [1048576, "1.0 MiB"], [5 * 2 ** 20, "5.0 MiB"], [150 * 2 ** 20, "150 MiB"],
    [3 * 2 ** 30, "3.0 GiB"], [2 ** 20 - 1, "1.0 MiB"], [-2048, "-2.0 KiB"],
  ];
  for (const [n, want] of cases) expect(bytes(n)).toBe(want);
});

test("duration", () => {
  const s = 1000;
  const m = 60 * s;
  const h = 60 * m;
  const cases: [number, string][] = [
    [0, "0ms"], [450, "450ms"], [s, "1s"], [59 * s, "59s"], [2 * m + 5 * s, "2m 5s"], [2 * m, "2m"],
    [h + 5 * m, "1h 5m"], [3 * h, "3h"], [76 * h, "3d 4h"], [-90 * s, "-1m 30s"],
  ];
  for (const [ms, want] of cases) expect(duration(ms)).toBe(want);
});

test("relative", () => {
  const now = Date.UTC(2026, 9, 19, 12);
  const s = 1000;
  const m = 60 * s;
  const h = 60 * m;
  const day = 24 * h;
  const cases: [number, string][] = [
    [-3 * s, "just now"], [5 * s, "just now"], [-30 * s, "30 seconds ago"], [-m, "1 minute ago"],
    [-5 * m, "5 minutes ago"], [-h, "1 hour ago"], [-23 * h, "23 hours ago"], [-48 * h, "2 days ago"],
    [-45 * day, "1 month ago"], [-400 * day, "1 year ago"], [2 * h, "in 2 hours"], [day, "in 1 day"],
  ];
  for (const [off, want] of cases) expect(relative(now + off, now)).toBe(want);
  expect(relative(new Date(now - m), new Date(now))).toBe("1 minute ago");
});

test("percent and thousands", () => {
  expect(percent(0.256)).toBe("26%");
  expect(percent(0.256, 1)).toBe("25.6%");
  expect(percent(1, -1)).toBe("100%");
  expect(percent(NaN)).toBe("-");
  const cases: [number, string][] = [
    [0, "0"], [12, "12"], [123, "123"], [1234, "1,234"], [12345, "12,345"], [123456, "123,456"],
    [1234567, "1,234,567"], [-1234567, "-1,234,567"], [-999, "-999"],
  ];
  for (const [n, want] of cases) expect(thousands(n)).toBe(want);
});
