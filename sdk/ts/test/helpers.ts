import { chmodSync, existsSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { spawnHost, type Host, type SpawnHostOptions } from "../src/index.ts";

const here = dirname(fileURLToPath(import.meta.url));

// `bin` is a tiny shell wrapper that runs the fake child with Bun, so the
// tests need neither an exec bit on a .ts file nor Bun on PATH.
const dir = mkdtempSync(join(tmpdir(), "tuimark-host-test-"));
export const fakeBin = join(dir, "tuimark");
writeFileSync(
  fakeBin,
  `#!/bin/sh\nexec '${process.execPath}' '${join(here, "fake-child.ts")}' "$@"\n`,
);
chmodSync(fakeBin, 0o755);

// Both test files share this directory, so it goes when the test process does.
process.on("exit", () => rmSync(dir, { recursive: true, force: true }));

let counter = 0;
const started: Host[] = [];

export interface Run {
  host: Host;
  /** Everything the fake child logged so far, in order. */
  records: () => any[];
  /** The messages the fake child received on fd 3 (without `argv`/`signal` records). */
  received: () => any[];
}

/** The environment that makes `fakeBin` follow `scenario`, logging to a fresh file. */
export function fakeEnv(scenario: object): { env: Record<string, string>; log: string } {
  const log = join(dir, `log-${++counter}.ndjson`);
  return {
    env: { ...(process.env as Record<string, string>), FAKE_SCENARIO: JSON.stringify(scenario), FAKE_LOG: log },
    log,
  };
}

export function readLog(log: string): any[] {
  if (!existsSync(log)) return [];
  return readFileSync(log, "utf8")
    .split("\n")
    .filter((l) => l !== "")
    .map((l) => JSON.parse(l));
}

/** Starts the fake child under `scenario`. Hosts are closed by `closeAll`. */
export async function start(
  scenario: object,
  options: SpawnHostOptions = {},
  file = "app.tui",
): Promise<Run> {
  const { env, log } = fakeEnv(scenario);
  const host = await spawnHost(file, { bin: fakeBin, env, ...options });
  started.push(host);
  const records = () => readLog(log);
  return { host, records, received: () => records().flatMap((r) => ("recv" in r ? [r.recv] : [])) };
}

export async function closeAll(): Promise<void> {
  await Promise.all(started.splice(0).map((h) => h.close()));
}

