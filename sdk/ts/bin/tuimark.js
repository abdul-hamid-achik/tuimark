#!/usr/bin/env node
// The `tuimark` command of @thelacanians/tuimark: runs the Tuimark binary of
// the release that matches this package's version, with the same arguments,
// stdio, exit code, and signals. The binary comes from TUIMARK_BINARY, the
// cache, or a checksum-verified download (see src/binary.ts). Plain JS for
// Node >= 18 and Bun; no dependencies and no postinstall script.

import { spawn } from "node:child_process";
import { fstatSync } from "node:fs";
import { constants } from "node:os";
import { isatty } from "node:tty";

async function loadResolver() {
  try {
    return await import("../dist/binary.js"); // built by `prepack`, shipped in the package
  } catch (error) {
    // In a clone, under Bun, the TypeScript source runs as it is.
    if (typeof Bun !== "undefined") return await import("../src/binary.ts");
    throw error;
  }
}

function fail(message) {
  process.stderr.write(`tuimark: ${message}\n`);
  process.exit(1);
}

let bin;
try {
  const { resolveBinary } = await loadResolver();
  bin = await resolveBinary({ log: (line) => process.stderr.write(`${line}\n`) });
} catch (error) {
  fail(error instanceof Error ? error.message : String(error));
}

const args = process.argv.slice(2);

// `tuimark host` talks to its parent on fd 3 and fd 4. Started directly by
// such a parent (a global install, node_modules/.bin), pass those two through.
// Only for `host`, and only for pipes or sockets. Node and Bun keep descriptors
// of their own at 3 and 4 that look the same (under npx or bunx, say), so the
// passthrough is a try: when the runtime refuses it, run without. Parents
// written in TypeScript should use spawnHost, which does not need this.
function isPipe(fd) {
  try {
    const st = fstatSync(fd);
    return st.isFIFO() || st.isSocket();
  } catch {
    return false;
  }
}
const stdio = [0, 1, 2];
if (args[0] === "host" && (isPipe(3) || isPipe(4))) {
  stdio.push(isPipe(3) ? 3 : "ignore", isPipe(4) ? 4 : "ignore");
}

// The handlers come before the child so that no signal is lost between the
// two: one that arrives early is passed on as soon as the child exists.
// SIGTERM and SIGHUP are for this process only, so they are forwarded. A
// terminal's SIGINT goes to the whole foreground process group, so the child
// already has it; only a SIGINT from elsewhere (a parent program, `kill`) is
// forwarded.
let child;
let early;
const forwarded = ["SIGTERM", "SIGHUP"];
function pass(signal) {
  if (child) child.kill(signal);
  else early = signal;
}
for (const signal of forwarded) process.on(signal, () => pass(signal));
process.on("SIGINT", () => {
  if (!isatty(0)) pass("SIGINT");
});

function start(options) {
  try {
    return spawn(bin, args, options);
  } catch (error) {
    return error;
  }
}
let started = start({ stdio });
if (started instanceof Error && stdio.length > 3) started = start({ stdio: [0, 1, 2] });
if (started instanceof Error) fail(`cannot start ${bin}: ${started.message}`);
child = started;
if (early) child.kill(early);

child.on("error", (error) => fail(`cannot start ${bin}: ${error.message}`));

child.on("exit", (code, signal) => {
  if (signal) {
    // Die from the same signal, so the caller sees what the child saw.
    for (const name of [...forwarded, "SIGINT"]) process.removeAllListeners(name);
    process.kill(process.pid, signal);
    setTimeout(() => process.exit(128 + (constants.signals[signal] ?? 0)), 100);
    return;
  }
  process.exit(code ?? 1);
});
