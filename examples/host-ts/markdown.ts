// markdownToBlocks: the reference producer of the kit's transcript blocks
// (kit/components/block.tui) from markdown, in TypeScript. It is an example,
// not part of the runtime or of @thelacanians/tuimark: Tuimark has no
// markdown tag, and a host that streams a model's reply turns the markdown
// into blocks itself and commits each finished block.
//
// A block is `{id, runs, code?, head?, quote?, item?, rule?}`: `runs` is an
// array of strings and `{text, class}` runs, and each flag is present (true)
// only when it holds, which the kit's k-block reads as false when missing.
// The run classes are the kit's: k-strong, k-em, and k-code.
//
// What it understands: paragraphs, ATX headings (#), fenced code (``` or
// ~~~), bullet and numbered lists (each item one block, nesting shown by
// indentation), block quotes (>), and rules (---, ***, ___); inside a block,
// **bold**, *emphasis* and `code`. Everything else is text.
//
// With {streaming: true} the markdown may end anywhere, as a model's reply
// does while it streams:
//   - an open fence is closed at the end of what has arrived, so the code so
//     far is a code block;
//   - an unclosed `**`, `*` or backtick stays text, so a half-written bold
//     is never styled to the end of the block;
//   - a last line that is only the start of a block marker (`#`, `-`, `>`,
//     `1.`, `` ` ``, ``` `` ```) is held back, so the tail never flashes a
//     stray "#" before the heading it will become.
// The last block of a streaming result is provisional: more text may extend
// it. Block `i` has id `i` in every call, so the blocks of a longer prefix
// start with those of a shorter one, apart from the last of the shorter.

export type Run = string | { text: string; class?: string };

export interface Block {
	id: number;
	runs: Run[];
	code?: true;
	head?: true;
	quote?: true;
	item?: true;
	rule?: true;
}

export interface MarkdownOptions {
	/** The markdown may end anywhere; see the header. */
	streaming?: boolean;
}

export const STRONG = "k-strong";
export const EM = "k-em";
export const CODE = "k-code";

const FENCE = /^( {0,3})(`{3,}|~{3,})(.*)$/;
const HEADING = /^ {0,3}(#{1,6})[ \t]+(.*?)[ \t#]*$/;
const RULE = /^ {0,3}([-*_])([ \t]*\1){2,}[ \t]*$/;
const BULLET = /^( *)([-*+])[ \t]+(.*)$/;
const NUMBERED = /^( *)(\d{1,9})[.)][ \t]+(.*)$/;
const QUOTE = /^ {0,3}>[ \t]?(.*)$/;
const PARTIAL_MARKER = /^ {0,3}(#{1,6}|[-*+>]|\d{1,9}[.)]?|`{1,2}|~{1,2}|-{1,2}|\*{1,2}|_{1,2})$/;

export function markdownToBlocks(md: string, options: MarkdownOptions = {}): Block[] {
	const streaming = options.streaming === true;
	const lines = md.replace(/\r\n?/g, "\n").split("\n");
	// The last element is the line still being written when the text does not
	// end with a line break.
	const unfinished = lines.length > 0 && lines[lines.length - 1] !== "";
	if (!unfinished) lines.pop();
	if (streaming && unfinished && PARTIAL_MARKER.test(lines[lines.length - 1] ?? "")) lines.pop();

	const blocks: Block[] = [];
	const push = (b: Omit<Block, "id">): void => {
		blocks.push({ id: blocks.length, ...b });
	};

	let para: string[] = [];
	const flush = (): void => {
		if (para.length > 0) push({ runs: inline(para.join(" ")) });
		para = [];
	};

	for (let i = 0; i < lines.length; i++) {
		const line = lines[i] as string;
		if (line.trim() === "") {
			flush();
			continue;
		}
		const fence = FENCE.exec(line);
		if (fence) {
			flush();
			const marker = fence[2] as string;
			const indent = (fence[1] as string).length;
			const body: string[] = [];
			i++;
			for (; i < lines.length; i++) {
				const l = lines[i] as string;
				const close = FENCE.exec(l);
				if (close && (close[2] as string)[0] === marker[0] && (close[2] as string).length >= marker.length && (close[3] as string).trim() === "") break;
				body.push(l.replace(new RegExp(`^ {0,${indent}}`), ""));
			}
			// Falling off the end is an open fence: it is closed here, in
			// either mode, so the code so far is a code block.
			push({ runs: [body.join("\n")], code: true });
			continue;
		}
		const heading = HEADING.exec(line);
		if (heading) {
			flush();
			push({ runs: inline(heading[2] as string), head: true });
			continue;
		}
		if (RULE.test(line)) {
			flush();
			push({ runs: [], rule: true });
			continue;
		}
		const quote = QUOTE.exec(line);
		if (quote) {
			flush();
			const parts = [quote[1] as string];
			while (i + 1 < lines.length) {
				const next = QUOTE.exec(lines[i + 1] as string);
				if (!next) break;
				parts.push(next[1] as string);
				i++;
			}
			push({ runs: inline(parts.join(" ").trim()), quote: true });
			continue;
		}
		const item = BULLET.exec(line) ?? NUMBERED.exec(line);
		if (item) {
			flush();
			const depth = Math.floor((item[1] as string).length / 2);
			const mark = /^\d/.test(item[2] as string) ? `${item[2] as string}.` : "•";
			const parts = [item[3] as string];
			// An indented line that starts no item continues it.
			while (i + 1 < lines.length) {
				const next = lines[i + 1] as string;
				if (next.trim() === "" || !/^ {2,}\S/.test(next) || BULLET.test(next) || NUMBERED.test(next)) break;
				parts.push(next.trim());
				i++;
			}
			push({ runs: inline(`${"  ".repeat(depth)}${mark} ${parts.join(" ")}`), item: true });
			continue;
		}
		para.push(line.trim());
	}
	flush();
	return blocks;
}

// inline splits a line into runs: **bold** (or __bold__), *em* (or _em_) and
// `code`. A delimiter with no closing partner is text.
export function inline(text: string): Run[] {
	const runs: Run[] = [];
	let plain = "";
	const flush = (): void => {
		if (plain !== "") runs.push(plain);
		plain = "";
	};
	let i = 0;
	while (i < text.length) {
		const c = text[i] as string;
		if (c === "`") {
			const end = text.indexOf("`", i + 1);
			if (end > i + 1) {
				flush();
				runs.push({ text: text.slice(i + 1, end), class: CODE });
				i = end + 1;
				continue;
			}
		} else if (c === "*" || c === "_") {
			const double = text[i + 1] === c;
			const mark = double ? c + c : c;
			const end = closing(text, i + mark.length, mark);
			// _ inside a word (snake_case) is not emphasis.
			const wordInside = c === "_" && i > 0 && /\w/.test(text[i - 1] as string);
			if (end > i + mark.length && !wordInside) {
				flush();
				runs.push({ text: text.slice(i + mark.length, end), class: double ? STRONG : EM });
				i = end + mark.length;
				continue;
			}
			if (double && !wordInside) {
				// An unclosed ** is text: both stars, so the second is not
				// taken for the start of an emphasis.
				plain += mark;
				i += 2;
				continue;
			}
		}
		plain += c;
		i++;
	}
	flush();
	return runs;
}

// closing finds the next `mark` at or after `from` that is not followed by
// the same character (so `**` is not taken for the end of a `*`), or -1.
function closing(text: string, from: number, mark: string): number {
	let at = text.indexOf(mark, from);
	while (at >= 0) {
		const after = text[at + mark.length];
		if (mark.length === 2 || after !== mark[0]) return at;
		at = text.indexOf(mark, at + 1);
	}
	return -1;
}
