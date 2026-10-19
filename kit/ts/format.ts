// format: numbers, sizes and times as strings ready for {path}.
//
// Copy this file into your project. It has no dependencies.
//
// Markup has no expressions, so a size or a duration is formatted in the
// parent and written to the store as text: ui.set("disk.text", bytes(n)).

// toFixed with a half rounding up, as the Go helper does, and no "-0.0".
function fixed(x: number, decimals: number): string {
  const s = x.toFixed(decimals);
  return /^-0(\.0*)?$/.test(s) ? s.slice(1) : s;
}

/**
 * 0 B, 999 B, 1.0 KiB, 1.5 MiB, 3.2 GiB: binary units, one decimal under 100
 * of a unit. A value that is not finite is "-".
 */
export function bytes(n: number): string {
  if (!Number.isFinite(n)) return "-";
  if (n < 0) return "-" + bytes(-n);
  if (n < 1024) return `${Math.trunc(n)} B`;
  const units = ["KiB", "MiB", "GiB", "TiB", "PiB", "EiB"];
  let v = n;
  let u = -1;
  while (v >= 1024 && u < units.length - 1) {
    v /= 1024;
    u++;
  }
  if (v >= 1023.5 && u < units.length - 1) {
    v /= 1024; // rounds up to the next unit
    u++;
  }
  // One decimal would print 100.0 from 99.95 up.
  return v >= 99.95 ? `${fixed(v, 0)} ${units[u]}` : `${fixed(v, 1)} ${units[u]}`;
}

/** A duration in milliseconds with its two largest units: 450ms, 3s, 2m 5s, 1h 5m, 3d 4h. Not finite: "-". */
export function duration(ms: number): string {
  if (!Number.isFinite(ms)) return "-";
  if (ms < 0) return "-" + duration(-ms);
  if (ms < 1000) return `${Math.trunc(ms)}ms`;
  let secs = Math.floor(ms / 1000);
  const days = Math.floor(secs / 86400);
  secs %= 86400;
  const hours = Math.floor(secs / 3600);
  secs %= 3600;
  const mins = Math.floor(secs / 60);
  secs %= 60;
  const pair = (a: number, au: string, b: number, bu: string) => (b === 0 ? `${a}${au}` : `${a}${au} ${b}${bu}`);
  if (days > 0) return pair(days, "d", hours, "h");
  if (hours > 0) return pair(hours, "h", mins, "m");
  if (mins > 0) return pair(mins, "m", secs, "s");
  return `${secs}s`;
}

/**
 * `t` relative to `now`: "just now" (under 10 s), "5 minutes ago", "in 2 days".
 * `now` is a parameter so a test, or a frame that must not change between
 * redraws, can fix it.
 */
export function relative(t: Date | number, now: Date | number = Date.now()): string {
  const d = Number(now) - Number(t);
  if (!Number.isFinite(d)) return "-";
  const future = d < 0;
  const abs = Math.abs(d);
  const s = 1000;
  const m = 60 * s;
  const h = 60 * m;
  const day = 24 * h;
  if (abs < 10 * s) return "just now";
  let n: number;
  let unit: string;
  if (abs < m) [n, unit] = [Math.floor(abs / s), "second"];
  else if (abs < h) [n, unit] = [Math.floor(abs / m), "minute"];
  else if (abs < day) [n, unit] = [Math.floor(abs / h), "hour"];
  else if (abs < 30 * day) [n, unit] = [Math.floor(abs / day), "day"];
  else if (abs < 365 * day) [n, unit] = [Math.floor(abs / (30 * day)), "month"];
  else [n, unit] = [Math.floor(abs / (365 * day)), "year"];
  if (n !== 1) unit += "s";
  return future ? `in ${n} ${unit}` : `${n} ${unit} ago`;
}

/** A fraction as a percentage: percent(0.256, 0) is "26%", percent(0.256, 1) is "25.6%". */
export function percent(fraction: number, decimals = 0): string {
  if (!Number.isFinite(fraction)) return "-";
  return fixed(fraction * 100, Math.max(0, decimals)) + "%";
}

/** An integer with comma separators: 1234567 is "1,234,567". */
export function thousands(n: number): string {
  if (!Number.isFinite(n)) return "-";
  const s = Math.trunc(Math.abs(n)).toString();
  let out = "";
  for (let i = 0; i < s.length; i++) {
    if (i > 0 && (s.length - i) % 3 === 0) out += ",";
    out += s[i];
  }
  return n < 0 ? "-" + out : out;
}
