// bun test: streaming a reply by blocks against a recording sink.
import { describe, expect, test } from "bun:test";
import type { Commit, Write } from "@thelacanians/tuimark";
import { streamReply } from "./stream.ts";

type Call = { kind: "commit"; commit: Commit } | { kind: "batch"; writes: Write[] };

function recorder() {
	const calls: Call[] = [];
	const buf: unknown[] = [];
	return {
		calls,
		// The text of each block, in the order a commit reads it from the
		// scratch path.
		committedText: () => buf,
		sink: {
			async commit(commit: Commit) {
				calls.push({ kind: "commit", commit });
				return { ok: true };
			},
			async batch(writes: Write[]) {
				calls.push({ kind: "batch", writes });
				for (const w of writes) if (w.path === "commit_buf" && w.value !== null) buf.push(w.value);
				return { ok: true };
			},
		},
	};
}

async function* chunks(text: string, size: number): AsyncGenerator<string> {
	for (let i = 0; i < text.length; i += size) {
		yield text.slice(i, i + size);
		await Promise.resolve();
	}
}

const reply = "# Title\n\nOne **two**.\n\n- a\n- b\n\n```\ncode\n```\n\nEnd.\n";

const texts = (blocks: unknown[]) =>
	blocks.map((b) => ((b as { runs: Array<string | { text: string }> }).runs.map((r) => (typeof r === "string" ? r : r.text)).join("")));

describe("streamReply", () => {
	test("commits every block exactly once, in order, whatever the chunk size", async () => {
		for (const size of [1, 3, 7, 1000]) {
			const rec = recorder();
			const result = await streamReply(rec.sink, { chunks: chunks(reply, size), signal: new AbortController().signal });
			expect(result).toEqual({ committed: 6, stopped: false });
			expect(texts(rec.committedText())).toEqual(["Title", "One two.", "• a", "• b", "code", "End."]);
			expect(rec.calls.filter((c) => c.kind === "commit")).toHaveLength(6);
		}
	});

	test("each commit reads the scratch path it was written to, and the last write clears the tail", async () => {
		const rec = recorder();
		await streamReply(rec.sink, { chunks: chunks(reply, 5), signal: new AbortController().signal });
		for (const call of rec.calls) {
			if (call.kind === "commit") expect(call.commit).toEqual({ template: "commit-block", with: "commit_buf as cb" });
		}
		const last = rec.calls.at(-1);
		expect(last).toEqual({
			kind: "batch",
			writes: [
				{ path: "commit_buf", value: null },
				{ path: "live.blocks", value: [] },
				{ path: "live.active", value: false },
			],
		});
	});

	test("a commit is sent before the write that moves the tail past it", async () => {
		const rec = recorder();
		await streamReply(rec.sink, { chunks: chunks(reply, 4), signal: new AbortController().signal });
		let committed = 0;
		for (const call of rec.calls) {
			if (call.kind === "commit") committed++;
			if (call.kind === "batch") {
				const tail = call.writes.find((w) => w.path === "live.blocks");
				if (tail !== undefined) {
					// The tail is the block with id `committed` or later: a block
					// is never in the tail after its commit was sent.
					const blocks = tail.value as Array<{ id: number }>;
					for (const b of blocks) expect(b.id).toBeGreaterThanOrEqual(committed);
				}
			}
		}
	});

	test("a cancel commits the partial tail and the stopped line, and ends the stream", async () => {
		const rec = recorder();
		const stop = new AbortController();
		async function* source(): AsyncGenerator<string> {
			yield "# Title\n\nOne **tw";
			yield "o** and more";
			stop.abort();
			yield "never read";
		}
		const result = await streamReply(rec.sink, { chunks: source(), signal: stop.signal, stoppedLine: "(cancelled)" });
		expect(result).toEqual({ committed: 2, stopped: true });
		expect(texts(rec.committedText())).toEqual(["Title", "One two and more"]);
		expect(rec.calls.at(-1)).toEqual({ kind: "commit", commit: { lines: ["(cancelled)"] } });
	});

	test("an unclosed fence is committed as the code so far when the stream is stopped", async () => {
		const rec = recorder();
		const stop = new AbortController();
		async function* source(): AsyncGenerator<string> {
			yield "text\n\n```\nlet a = 1;\nlet";
			stop.abort();
			yield "x";
		}
		await streamReply(rec.sink, { chunks: source(), signal: stop.signal });
		expect(texts(rec.committedText())).toEqual(["text", "let a = 1;\nlet"]);
	});

	test("a throwing stream still commits what arrived", async () => {
		const rec = recorder();
		async function* source(): AsyncGenerator<string> {
			yield "one\n\ntwo";
			throw new Error("model failed");
		}
		await expect(streamReply(rec.sink, { chunks: source(), signal: new AbortController().signal })).rejects.toThrow("model failed");
		expect(texts(rec.committedText())).toEqual(["one", "two"]);
	});
});
