// Streaming a reply into the scrollback by blocks (SPEC §26.15.4): the
// markdown arrives in chunks, each finished block is committed once (a
// template's copy, above the live region), and only the unfinished tail
// block stays in the region (`live.blocks`, drawn by the kit's k-block).
//
// The commit of a block and the write that moves the tail are sent
// together, so the child draws them as one frame: it prints the commit
// before the frame that shows the write. On a cancel or a quit the partial
// tail is committed too, before the caller replies, so what the user saw is
// never lost with the region.
import type { Commit, Write } from "@thelacanians/tuimark";
import { type Block, markdownToBlocks } from "./markdown.ts";

/** The part of a Host that streaming uses; a test passes a fake. */
export interface Sink {
	commit(commit: Commit): Promise<unknown>;
	batch(writes: Write[]): Promise<unknown>;
}

export interface StreamOptions {
	/** Chunks of the reply, as a model's stream would deliver them. */
	chunks: AsyncIterable<string>;
	/** Stop streaming: the tail is committed and the stream ends. */
	signal: AbortSignal;
	/** A line committed after a reply that was stopped. */
	stoppedLine?: string;
}

export interface StreamResult {
	/** How many blocks were committed. */
	committed: number;
	/** Whether the stream was stopped before its end. */
	stopped: boolean;
}

/** The commit of one block: the template of inline.tui, over the scratch path `commit_buf`. */
export function blockCommit(): Commit {
	return { template: "commit-block", with: "commit_buf as cb" };
}

export async function streamReply(ui: Sink, options: StreamOptions): Promise<StreamResult> {
	let buf = "";
	let committed = 0;

	// flush commits `blocks`, then moves the tail to `tail`. Each block is
	// written to the scratch path `commit_buf` and committed through the
	// wrapper template that reads it (§26.15.4, a copy reads the item
	// through a `with` alias); the last write clears the path and sets the
	// tail. All of it is sent before the first ack is awaited, so the child
	// reads it as one run of messages: the commits are printed, in order,
	// before the frame that shows the tail.
	const flush = async (blocks: Block[], tail: Block[]): Promise<void> => {
		const sent: Promise<unknown>[] = [];
		for (const block of blocks) {
			sent.push(ui.batch([{ path: "commit_buf", value: block }]));
			sent.push(ui.commit(blockCommit()));
			committed++;
		}
		sent.push(
			ui.batch([
				{ path: "commit_buf", value: null },
				{ path: "live.blocks", value: tail },
				{ path: "live.active", value: tail.length > 0 },
			]),
		);
		await Promise.all(sent);
	};

	let stopped = false;
	try {
		for await (const chunk of options.chunks) {
			if (options.signal.aborted) {
				stopped = true;
				break;
			}
			buf += chunk;
			const blocks = markdownToBlocks(buf, { streaming: true });
			// Everything before the last block is finished: commit it, and
			// show the last one in the region.
			await flush(blocks.slice(committed, -1), blocks.slice(-1));
		}
		stopped ||= options.signal.aborted;
	} finally {
		// The end of the reply, a cancel, or a throw: what is left goes
		// into the scrollback before the region forgets it.
		await flush(markdownToBlocks(buf, { streaming: stopped }).slice(committed), []);
		if (stopped && options.stoppedLine !== undefined) await ui.commit({ lines: [options.stoppedLine] });
	}
	return { committed, stopped };
}
