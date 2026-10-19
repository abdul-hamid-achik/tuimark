// parent.ts: app.tui driven by a Bun parent through @thelacanians/tuimark.
//
// The view lives entirely in app.tui and theme.tcss. `tuimark host` lays it
// out and paints it in this terminal; this file keeps the data and answers
// the named actions the keymap and the widgets fire: select, open, search,
// refresh, and quit. It never reads or writes the terminal itself.
//
// Run it with: bun parent.ts
//
// spawnHost runs the tuimark binary of this package's release (downloaded
// once and checked against the release's checksums). To use another build,
// set TUIMARK_BINARY=/path/to/tuimark or pass { bin: "/path/to/tuimark" }.

import { spawnHost } from "@thelacanians/tuimark";

type Item = { id: string; title: string };

const sample = (await Bun.file("sample.json").json()) as { items: Item[] };
const all: Item[] = sample.items;

const ui = await spawnHost("app.tui", { data: "sample.json", replyTimeout: "5s" });

// show writes the list that matches q, and the line and count that go with it.
async function show(q: string): Promise<void> {
	const needle = q.trim().toLowerCase();
	const hits = all.filter((it) => needle === "" || it.title.toLowerCase().includes(needle));
	await ui.batch([
		{ path: "items", value: hits },
		{ path: "count", value: hits.length },
		{ path: "status", value: needle === "" ? "all items" : `${hits.length} match "${q.trim()}"` },
	]);
}

ui.on("select", async (ev) => {
	await ui.set("status", `selected ${String(ev.keys.item)}`);
});

ui.on("open", async (ev) => {
	await ui.set("status", `opened ${String(ev.keys.item)}`);
});

ui.on("search", async (ev) => {
	await show(typeof ev.value === "string" ? ev.value : "");
});

ui.on("refresh", async () => {
	await ui.set("query", "");
	await show("");
});

ui.on("quit", async () => "quit");

const exit = await ui.exited;
process.exit(exit.code);
