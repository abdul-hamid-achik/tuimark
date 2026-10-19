#!/usr/bin/env bun
// examples/host-ts: a TypeScript parent, run by Bun, that drives a Tuimark
// view over `tuimark host` (the host protocol, protocol 1) through
// @thelacanians/tuimark (sdk/ts, SPEC §19.2). It is a parent, not a second host:
// `tuimark` lays out, paints, and decides everything; this script only
// answers the one host action of view.tui ("inc") by writing to the store.
//
// The package does the protocol's work for a parent (SPEC §19.1): it passes
// fd 3 and fd 4 and reads them as Bun needs, checks `protocol`, forwards
// SIGTERM and SIGHUP to the child, never reads or writes the terminal, and
// treats a child exit without an "exit" message as an error. What stays here
// is the application, and the rule that the parent exits with the child's
// code, after the child has exited and the terminal is its own again.
//
// Usage: bun run parent.ts [path/to/tuimark]
// (defaults to $TUIMARK_BIN, then "tuimark" on PATH)

import { HostError, ProtocolVersionError, spawnHost } from "@thelacanians/tuimark";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const bin = process.argv[2] ?? process.env.TUIMARK_BIN ?? "tuimark";

let count = 0; // the parent owns the model; the view shows it
const errors: string[] = [];

// Nothing is printed until the child has exited: it owns the terminal.
function finish(problem: string, code: number): never {
	console.error(`host-ts: ${problem}`);
	for (const e of errors) {
		console.error(`host-ts: protocol error: ${e}`);
	}
	process.exit(code === 0 ? 1 : code);
}

let ui: Awaited<ReturnType<typeof spawnHost>>;
try {
	ui = await spawnHost(join(here, "view.tui"), {
		data: join(here, "sample.json"),
		bin,
		onProtocolError: (m) => errors.push(m.error),
	});
} catch (error) {
	finish(error instanceof Error ? error.message : String(error), error instanceof HostError ? (error.code ?? 1) : 1);
}

// Every event gets a reply when the handler returns. Writes made before it
// returns are applied before the reply, so the frame drawn after it shows
// both.
ui.on("inc", async () => {
	count++;
	await ui.batch([
		{ path: "count", value: count },
		{ path: "last", value: true },
	]);
});

try {
	const exit = await ui.exited;
	const detail = exit.reason === "signal" ? ` ${exit.signal}` : exit.reason === "error" ? `: ${exit.error}` : "";
	console.error(`host-ts: the view ended (${exit.reason}${detail}); count=${count}`);
	for (const e of errors) {
		console.error(`host-ts: protocol error: ${e}`);
	}
	process.exit(exit.code);
} catch (error) {
	if (error instanceof ProtocolVersionError) {
		finish(`unknown protocol ${String(error.protocol)}`, error.code ?? 1);
	}
	finish(error instanceof Error ? error.message : String(error), error instanceof HostError ? (error.code ?? 1) : 1);
}
