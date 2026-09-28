#!/usr/bin/env bun
// examples/host-ts: a TypeScript parent, run by Bun, that drives a Tuimark
// view over `tuimark host` (the host protocol, protocol 1). It is a
// parent, not a second host: `tuimark` lays out, paints, and decides
// everything; this script only speaks the protocol and answers the one
// host action of view.tui ("inc") by writing to the store.
//
// Transport (Bun): stdio ["inherit", "inherit", "inherit", "pipe", "pipe"]
// gives the child the terminal on fds 0-2, unchanged, and gives this
// process fd 3 (parent -> child) and fd 4 (child -> parent) as raw,
// non-blocking descriptor numbers: the ends of Unix socket pairs, not
// pipes. fs.readSync and fs.createReadStream fail on them with EAGAIN, so
// fd 4 is read with Bun.file(fd).stream() and fd 3 is written with
// fs.createWriteStream("", { fd }), whose end() gives the child end of
// file on fd 3.
//
// Rules for parents, which this script follows:
// - It never reads or writes the terminal while the child runs (no
//   process.stdin, no console output): the child owns fds 0 and 1. It
//   reports only after the child has exited.
// - It forwards SIGTERM and SIGHUP to the child, waits for the child's
//   exit, and exits with the child's code.
// - To end the child itself it sends SIGTERM, waits 2 seconds, and only
//   then sends SIGKILL.
// - It checks `protocol` in "ready", and treats a child exit without an
//   "exit" message as an error.
//
// Usage: bun run parent.ts [path/to/tuimark]
// (defaults to $TUIMARK_BIN, then "tuimark" on PATH)

import { createWriteStream } from "node:fs";
import { constants } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const tuimarkBin = process.argv[2] ?? process.env.TUIMARK_BIN ?? "tuimark";

type Msg = Record<string, unknown>;

const proc = Bun.spawn({
	cmd: [tuimarkBin, "host", join(here, "view.tui"), "--data", join(here, "sample.json")],
	stdio: ["inherit", "inherit", "inherit", "pipe", "pipe"],
});

const toChild = createWriteStream("", { fd: proc.stdio[3] as number });
// A write after the child exited fails with EPIPE; the exit is handled
// below, so the stream's error is not.
toChild.on("error", () => {});

function send(msg: Msg): void {
	toChild.write(`${JSON.stringify(msg)}\n`);
}

// stopChild ends the child the way the protocol asks: SIGTERM, then
// SIGKILL only if it is still running 2 seconds later (the child restores
// the terminal within its 1-second signal grace; a SIGKILL cannot).
function stopChild(): void {
	proc.kill("SIGTERM");
	const timer = setTimeout(() => proc.kill("SIGKILL"), 2000);
	proc.exited.then(() => clearTimeout(timer));
}

let ready: Msg | undefined;
let exit: Msg | undefined;
let problem = "";
const errors: string[] = [];
let count = 0; // the parent owns the model; the view shows it

function handle(msg: Msg): void {
	switch (msg.type) {
		case "ready":
			ready = msg;
			if (msg.protocol !== 1) {
				problem = `unknown protocol ${String(msg.protocol)}`;
				stopChild();
			}
			break;
		case "event":
			if (msg.action === "inc") {
				count++;
				// Writes sent before the reply are applied before it, so the
				// frame drawn after the reply shows both.
				send({ type: "set", path: "count", value: count });
				send({ type: "set", path: "last", value: true });
			}
			// Every event gets a reply, known action or not, so the loop is
			// never held until the reply timeout.
			send({ type: "reply", seq: msg.seq });
			break;
		case "error":
			errors.push(String(msg.error));
			break;
		case "exit":
			exit = msg;
			break;
		default:
			break; // "ack": this parent sends no message with an id
	}
}

async function readMessages(fd: number): Promise<void> {
	const reader = Bun.file(fd).stream().getReader();
	const decoder = new TextDecoder();
	let buf = "";
	for (;;) {
		const { value, done } = await reader.read();
		if (done) {
			break;
		}
		buf += decoder.decode(value, { stream: true });
		for (let nl = buf.indexOf("\n"); nl >= 0; nl = buf.indexOf("\n")) {
			const line = buf.slice(0, nl);
			buf = buf.slice(nl + 1);
			try {
				handle(JSON.parse(line));
			} catch {
				// Not a message: only a child that died mid-line writes one.
			}
		}
	}
}

for (const sig of ["SIGTERM", "SIGHUP"] as const) {
	process.on(sig, () => proc.kill(sig));
}

await readMessages(proc.stdio[4] as number);
const exitCode = await proc.exited;
toChild.end();

// The terminal is this process's again: report.
const signal = proc.signalCode;
let code = signal ? 128 + (constants.signals[signal as keyof typeof constants.signals] ?? 0) : exitCode;
if (!ready) {
	problem ||= "the child never sent ready";
} else if (!exit) {
	problem ||= "the child exited without an exit message";
}
if (problem) {
	console.error(`host-ts: ${problem}`);
	if (code === 0) {
		code = 1;
	}
} else {
	const detail = exit?.reason === "signal" ? ` ${String(exit.signal)}` : exit?.reason === "error" ? `: ${String(exit.error)}` : "";
	console.error(`host-ts: the view ended (${String(exit?.reason)}${detail}); count=${count}`);
}
for (const e of errors) {
	console.error(`host-ts: protocol error: ${e}`);
}
process.exit(code);
