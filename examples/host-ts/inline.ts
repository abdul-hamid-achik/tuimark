#!/usr/bin/env bun
// examples/host-ts/inline.ts: a TypeScript parent that runs a chat inline and
// streams a reply into the terminal's own scrollback, block by block (SPEC
// v0.4d §26.15.4, §19.2). The child, `tuimark host inline.tui --inline`, draws
// the screen as a live region below the shell's prompt; this program
//
//   - answers `composer_submit` by clearing the draft, committing the
//     message as a line and a tool receipt as a template's copy, and
//     streaming a reply (a scripted "model", markdown in small chunks);
//   - commits each finished block of the reply as soon as the next one
//     begins (markdownToBlocks, then `commit`), and keeps only the
//     unfinished tail in the region;
//   - on `cancel` (esc) commits the partial tail and stops the reply;
//   - on `leave` (ctrl+q, ctrl+c) commits the partial tail, then "bye", and
//     only then replies "quit", so the last words are above the cursor when
//     the region is erased.
//
// It is an example, not part of the runtime. Usage: bun run inline.ts
// [path/to/tuimark] (defaults to $TUIMARK_BIN, then "tuimark" on PATH).
// HOST_TS_DELAY_MS sets the pause between the model's chunks (default 25).

import { HostError, ProtocolVersionError, spawnHost } from "@thelacanians/tuimark";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { type StreamResult, streamReply } from "./stream.ts";

const here = dirname(fileURLToPath(import.meta.url));
const bin = process.argv[2] ?? process.env.TUIMARK_BIN ?? "tuimark";
const delay = Number(process.env.HOST_TS_DELAY_MS ?? 25);

const errors: string[] = [];

function finish(problem: string, code: number): never {
	console.error(`host-ts: ${problem}`);
	for (const e of errors) console.error(`host-ts: protocol error: ${e}`);
	process.exit(code === 0 ? 1 : code);
}

// The scripted model: markdown that has every kind of block, delivered in
// chunks of a few words with a pause between them, as a stream would be.
function reply(question: string): string {
	return [
		"# A reply, streamed",
		"",
		"Streaming **by blocks** keeps the scrollback clean: a finished paragraph is committed *once*, and only the unfinished tail stays in the live region, drawn with `k-block`.",
		"",
		"- a finished block is committed above the region",
		"- the tail is redrawn as it grows",
		"",
		"```ts",
		"for (const block of finished) {",
		"\tawait ui.commit(blockCommit());",
		"}",
		"```",
		"",
		"> Commit per block, not per token.",
		"",
		"---",
		"",
		`You asked: \`${question.replaceAll("`", "'")}\`.`,
		"",
	].join("\n");
}

async function* model(text: string): AsyncGenerator<string> {
	const words = text.match(/\S+\s*|\s+/g) ?? [];
	for (let i = 0; i < words.length; i += 2) {
		yield words.slice(i, i + 2).join("");
		await new Promise((resolve) => setTimeout(resolve, delay));
	}
}

let ui: Awaited<ReturnType<typeof spawnHost>>;
try {
	ui = await spawnHost(join(here, "inline.tui"), {
		data: join(here, "inline.json"),
		bin,
		inline: true,
		onProtocolError: (m) => errors.push(m.error),
	});
} catch (error) {
	finish(error instanceof Error ? error.message : String(error), error instanceof HostError ? (error.code ?? 1) : 1);
}

let turn: { stop: AbortController; done: Promise<StreamResult> } | undefined;
let blocks = 0;

ui.on("composer_submit", async (ev) => {
	const text = String(ev.value ?? "").trim();
	if (text === "" || turn !== undefined) return;
	// The draft is cleared before the reply (enter changes nothing in a
	// textarea), and the composer is off while the reply streams.
	await ui.batch([
		{ path: "composer.draft", value: "" },
		{ path: "composer.disabled", value: true },
		{ path: "status", value: "streaming - esc cancels" },
	]);
	await ui.commit({ lines: [[{ text: "You: ", class: "k-strong" }, text]] });
	await ui.set("receipt", {
		title: "search · grep -c TODO src",
		status: "exit 0 · 0.1s",
		ok: true,
		output: "src/app.ts:\t3\nsrc/markdown.ts:\t1",
	});
	await ui.commit({ template: "k-receipt", with: "receipt as r" });
	const stop = new AbortController();
	const done = streamReply(ui, { chunks: model(reply(text)), signal: stop.signal, stoppedLine: "(cancelled)" }).then(async (result) => {
		blocks += result.committed;
		turn = undefined;
		await ui.batch([
			{ path: "composer.disabled", value: false },
			{ path: "status", value: "ready" },
		]);
		return result;
	});
	turn = { stop, done };
	// Not awaited: the handler returns, so keys (esc) keep working while the
	// reply streams.
	done.catch((error: unknown) => errors.push(error instanceof Error ? error.message : String(error)));
});

ui.on("cancel", () => {
	turn?.stop.abort();
});

ui.on("leave", async () => {
	// A quit mid-reply commits the partial tail before the reply, which is
	// what ends the session: the child prints a commit before it leaves.
	if (turn !== undefined) {
		turn.stop.abort();
		await turn.done.catch(() => undefined);
	}
	await ui.commit({ lines: ["bye"] });
	return "quit";
});

try {
	const exit = await ui.exited;
	const detail = exit.reason === "signal" ? ` ${exit.signal}` : exit.reason === "error" ? `: ${exit.error}` : "";
	console.error(`host-ts: the chat ended (${exit.reason}${detail}); ${blocks} blocks committed`);
	for (const e of errors) console.error(`host-ts: protocol error: ${e}`);
	process.exit(exit.code);
} catch (error) {
	if (error instanceof ProtocolVersionError) {
		finish(`unknown protocol ${String(error.protocol)}`, error.code ?? 1);
	}
	finish(error instanceof Error ? error.message : String(error), error instanceof HostError ? (error.code ?? 1) : 1);
}
